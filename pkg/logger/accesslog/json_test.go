package accesslog

import (
	"bytes"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/mojatter/io2"
	"github.com/valyala/fasthttp"
)

func TestAppendJSONString(t *testing.T) {
	testCases := []struct {
		caseName string
		in       string
		want     string
	}{
		{caseName: "empty", in: "", want: `""`},
		{caseName: "ascii", in: "hello", want: `"hello"`},
		{caseName: "double_quote", in: `say "hi"`, want: `"say \"hi\""`},
		{caseName: "backslash", in: `a\b`, want: `"a\\b"`},
		{caseName: "newline", in: "a\nb", want: `"a\nb"`},
		{caseName: "tab", in: "a\tb", want: `"a\tb"`},
		{caseName: "cr", in: "a\rb", want: `"a\rb"`},
		{caseName: "backspace", in: "a\bb", want: `"a\bb"`},
		{caseName: "formfeed", in: "a\fb", want: `"a\fb"`},
		{caseName: "control", in: "a\x01b", want: `"a\u0001b"`},
		{caseName: "delete_passthrough", in: "a\x7fb", want: "\"a\x7fb\""},
		{caseName: "non_ascii_utf8", in: "日本語", want: `"日本語"`},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			got := string(appendJSONString(nil, []byte(tc.in)))
			if got != tc.want {
				t.Errorf("appendJSONString(%q) = %q; want %q", tc.in, got, tc.want)
			}
			// The output must always be a valid JSON string literal.
			var decoded string
			if err := json.Unmarshal([]byte(got), &decoded); err != nil {
				t.Errorf("invalid JSON %q: %v", got, err)
			}
		})
	}
}

func TestAppendISOTime(t *testing.T) {
	tokyo, _ := time.LoadLocation("Asia/Tokyo")
	chicago, _ := time.LoadLocation("America/Chicago")
	india, _ := time.LoadLocation("Asia/Kolkata")

	testCases := []struct {
		caseName string
		date     time.Time
		want     string
	}{
		{
			caseName: "utc",
			date:     time.Date(2026, 4, 11, 1, 19, 13, 0, time.UTC),
			want:     "2026-04-11T01:19:13Z",
		},
		{
			caseName: "tokyo",
			date:     time.Date(2026, 4, 11, 10, 19, 13, 0, tokyo),
			want:     "2026-04-11T10:19:13+09:00",
		},
		{
			caseName: "chicago",
			date:     time.Date(2026, 4, 10, 20, 19, 13, 0, chicago),
			want:     "2026-04-10T20:19:13-05:00",
		},
		{
			caseName: "india_half_hour_offset",
			date:     time.Date(2026, 4, 11, 6, 49, 13, 0, india),
			want:     "2026-04-11T06:49:13+05:30",
		},
		{
			caseName: "year_max",
			date:     time.Date(2999, 12, 31, 23, 59, 59, 0, time.UTC),
			want:     "2999-12-31T23:59:59Z",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			got := string(appendISOTime(nil, tc.date))
			if got != tc.want {
				t.Errorf("appendISOTime = %q; want %q", got, tc.want)
			}
		})
	}
}

func TestAccessLog_JSON(t *testing.T) {
	timeNowOrg := timeNow
	defer func() { timeNow = timeNowOrg }()

	// fasthttp.RequestCtx exposes Time() but no setter, so the request time
	// is the zero value here. timeNow is mocked to make duration_us
	// deterministic against that zero value.
	timeNow = func() time.Time {
		var zeroTime time.Time
		return zeroTime.Add(1234 * time.Microsecond)
	}
	wantTime := "0001-01-01T00:00:00Z"

	remoteAddr := &net.TCPAddr{IP: net.IPv4(10, 1, 2, 3), Port: 51002}

	type expected struct {
		Time          string `json:"time"`
		RemoteAddr    string `json:"remote_addr"`
		ClientIP      string `json:"client_ip"`
		RemoteUser    string `json:"remote_user"`
		Method        string `json:"method"`
		URI           string `json:"uri"`
		Proto         string `json:"proto"`
		Scheme        string `json:"scheme"`
		Host          string `json:"host"`
		Status        int    `json:"status"`
		Size          int    `json:"size"`
		BytesReceived int    `json:"bytes_received"`
		DurationUs    int    `json:"duration_us"`
		Referer       string `json:"referer"`
		UserAgent     string `json:"user_agent"`
	}

	testCases := []struct {
		caseName string
		setup    func(ctx *fasthttp.RequestCtx)
		want     expected
	}{
		{
			caseName: "minimal_get",
			setup: func(ctx *fasthttp.RequestCtx) {
				ctx.Request.SetRequestURI("/path?foo=bar")
				ctx.Request.SetHost("localhost")
				ctx.Response.SetBody([]byte("body"))
				ctx.Response.Header.SetContentLength(4)
			},
			want: expected{
				RemoteAddr:    "10.1.2.3:51002",
				ClientIP:      "10.1.2.3",
				Method:        "GET",
				URI:           "/path?foo=bar",
				Proto:         "HTTP/1.1",
				Scheme:        "http",
				Host:          "localhost",
				Status:        200,
				Size:          4,
				BytesReceived: 0,
				DurationUs:    1234,
			},
		},
		{
			caseName: "with_user_referer_ua_quoted",
			setup: func(ctx *fasthttp.RequestCtx) {
				ctx.Request.SetRequestURI("/")
				ctx.Request.SetHost("example.com")
				ctx.URI().SetUsername("alice")
				ctx.Request.Header.Set("Referer", `http://example.com/"quoted"`)
				ctx.Request.Header.Set("User-Agent", `Mozilla/5.0 (back\slash)`)
			},
			want: expected{
				RemoteAddr: "10.1.2.3:51002",
				ClientIP:   "10.1.2.3",
				RemoteUser: "alice",
				Method:     "GET",
				URI:        "/",
				Proto:      "HTTP/1.1",
				Scheme:     "http",
				Host:       "example.com",
				Status:     200,
				Referer:    `http://example.com/"quoted"`,
				UserAgent:  `Mozilla/5.0 (back\slash)`,
				DurationUs: 1234,
			},
		},
		{
			caseName: "x_forwarded_for_left_most",
			setup: func(ctx *fasthttp.RequestCtx) {
				ctx.Request.SetRequestURI("/")
				ctx.Request.SetHost("localhost")
				ctx.Request.Header.Set("X-Forwarded-For", "203.0.113.7, 198.51.100.1, 10.0.0.1")
			},
			want: expected{
				RemoteAddr: "10.1.2.3:51002",
				ClientIP:   "203.0.113.7",
				Method:     "GET",
				URI:        "/",
				Proto:      "HTTP/1.1",
				Scheme:     "http",
				Host:       "localhost",
				Status:     200,
				DurationUs: 1234,
			},
		},
	}

	wroteCh := make(chan bool, 1)
	buf := new(bytes.Buffer)
	out := &io2.Delegator{
		WriteFunc: func(p []byte) (n int, err error) {
			n, err = buf.Write(p)
			wroteCh <- true
			return
		},
	}

	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			buf.Reset()
			ctx := &fasthttp.RequestCtx{}
			ctx.SetRemoteAddr(remoteAddr)
			tc.setup(ctx)

			cfg := config.Config{AccessLog: config.AccessLog{
				Format: FormatJSON,
				// Flush every Log() so wroteCh fires immediately.
				BufferSize: 1,
			}}
			l, err := newAccessLog(&logger.NopRotator{Writer: out}, cfg)
			if err != nil {
				t.Fatalf("newAccessLog: %v", err)
			}
			defer l.Close()

			l.Collect(ctx)
			l.Log(ctx)
			<-wroteCh

			line := strings.TrimRight(buf.String(), "\n")
			var got expected
			if err := json.Unmarshal([]byte(line), &got); err != nil {
				t.Fatalf("invalid JSON %q: %v", line, err)
			}
			tc.want.Time = wantTime
			if got != tc.want {
				t.Errorf("\n got: %+v\nwant: %+v\nraw: %s", got, tc.want, line)
			}
		})
	}
}
