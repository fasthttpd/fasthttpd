package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mojatter/tree"
	"github.com/valyala/fasthttp"
	"go.yaml.in/yaml/v3"
)

func TestUnmarshalYAMLPath(t *testing.T) {
	got, err := UnmarshalYAMLPath("testdata/full.yaml")
	if err != nil {
		t.Fatal(err)
	}
	want := []Config{
		{
			Host:   "localhost",
			Listen: ":8080",
			Root:   "./public",
			Server: Server{
				Name:            "fasthttpd",
				ReadBufferSize:  4096,
				WriteBufferSize: 4096,
			},
			SSL: SSL{}.SetDefaults(),
			Log: Log{
				Output:   "logs/error.log",
				Flags:    []string{"date", "time"},
				Rotation: Rotation{}.SetDefaults(),
			},
			AccessLog: AccessLog{
				Output:        "logs/access.log",
				Format:        `%h %l %u %t "%r" %>s %b`,
				BufferSize:    4096,
				FlushInterval: 1000,
				Rotation:      Rotation{}.SetDefaults(),
			},
			ErrorPages: map[string]string{
				"404": "/err/404.html",
				"5xx": "/err/5xx.html",
			},
			Filters: map[string]tree.Map{
				"auth": {
					"type": tree.ToValue("basicAuth"),
					"users": tree.Array{
						tree.Map{
							"name":   tree.ToValue("fast"),
							"secret": tree.ToValue("httpd"),
						},
					},
					"usersFile": tree.ToValue("./users.yaml"),
				},
				"cache": {
					"type": tree.ToValue("header"),
					"response": tree.Map{
						"set": tree.Map{
							"Cache-Control": tree.ToValue("private, max-age=3600"),
						},
					},
				},
			},
			Handlers: map[string]tree.Map{
				"static": {
					"type":               tree.ToValue("fs"),
					"indexNames":         tree.ToArrayValues("index.html"),
					"compress":           tree.ToValue(true),
					"compressRoot":       tree.ToValue("./compressed"),
					"generateIndexPages": tree.ToValue(false),
				},
				"static-overwrite": {
					"type":       tree.ToValue("fs"),
					"indexNames": tree.ToArrayValues("index.html"),
					"root":       tree.ToValue("./public-overwrite"),
				},
				"hello": {
					"type": tree.ToValue("content"),
					"headers": tree.Map{
						"Content-Type": tree.ToValue("text/plain; charset=utf-8"),
					},
					"body": tree.ToValue("Hello FastHttpd"),
					"conditions": tree.Array{
						tree.Map{
							"path": tree.ToValue("/hello/world"),
							"body": tree.ToValue("Hello world"),
						},
						tree.Map{
							"queryStringContains": tree.ToValue("time=morning"),
							"body":                tree.ToValue("Good morning FastHttpd"),
						},
						tree.Map{
							"percentage": tree.ToValue(10),
							"body":       tree.ToValue("10% hit FastHttpd"),
						},
					},
				},
			},
			Routes: []Route{
				{
					Methods:       []string{"PUT", "DELETE", "CONNECT", "OPTIONS", "TRACE", "PATCH"},
					Status:        405,
					StatusMessage: "Method not allowed",
				}, {
					Path:    "/",
					Match:   MatchEqual,
					Handler: "static",
				}, {
					Path:    "/redirect-external",
					Match:   MatchEqual,
					Rewrite: "http://example.com/",
					Status:  302,
				}, {
					Path:                     "/redirect-internal",
					Match:                    MatchEqual,
					Rewrite:                  "/internal?foo=bar",
					RewriteAppendQueryString: true,
					Status:                   302,
				}, {
					Methods:        []string{"GET", "HEAD"},
					Filters:        []string{"cache"},
					Path:           `.*\.(js|css|jpg|png|gif|ico)$`,
					Match:          MatchRegexp,
					Handler:        "static-overwrite",
					NextIfNotFound: true,
				}, {
					Methods: []string{"GET", "HEAD"},
					Filters: []string{"cache"},
					Path:    `.*\.(js|css|jpg|png|gif|ico)$`,
					Match:   MatchRegexp,
					Handler: "static",
				}, {
					Path:    "^/view/(.+)",
					Match:   MatchRegexp,
					Rewrite: "/view?id=$1",
				}, {
					Filters: []string{"auth"},
					Handler: "hello",
				},
			},
			RoutesCache: RoutesCache{
				Enable: true,
				Expire: 60000,
			},
			ShutdownTimeout: DefaultShutdownTimeout,
		}, {
			Host:      "localhost",
			Listen:    ":8443",
			Log:       Log{}.SetDefaults(),
			AccessLog: AccessLog{}.SetDefaults(),
			SSL: SSL{
				CertFile: "./ssl/localhost.crt",
				KeyFile:  "./ssl/localhost.key",
			},
			Server: Server{Name: "fasthttpd"},
			Handlers: map[string]tree.Map{
				"backend": {
					"type": tree.ToValue("proxy"),
					"url":  tree.ToValue("http://localhost:8080"),
				},
			},
			Routes: []Route{
				{
					Path:    "/",
					Handler: "backend",
				},
			},
			ShutdownTimeout: DefaultShutdownTimeout,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v\nwant %#v", got, want)
	}
}

func TestUnmarshalYAMLPath_Errors(t *testing.T) {
	invalidYaml, err := os.CreateTemp("", "*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(invalidYaml.Name())

	if err := os.WriteFile(invalidYaml.Name(), []byte(":invalid yaml"), os.ModePerm); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		path   string
		errstr string
	}{
		{
			path:   "testdata/not-found.yaml",
			errstr: "open testdata/not-found.yaml: no such file or directory",
		}, {
			path:   invalidYaml.Name(),
			errstr: "yaml: unmarshal errors",
		},
	}
	for i, test := range tests {
		_, err := UnmarshalYAMLPath(test.path)
		if err == nil {
			t.Fatalf("tests[%d] no error", i)
		}
		if !strings.HasPrefix(err.Error(), test.errstr) {
			t.Errorf("tests[%d] got %q; want %q", i, err.Error(), test.errstr)
		}
	}
}

func TestUnmarshalYAMLPath_Include(t *testing.T) {
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(currentDir) //nolint:errcheck

	if err := os.Chdir("testdata"); err != nil {
		t.Fatal(err)
	}

	got, err := UnmarshalYAMLPath("include.yaml")
	if err != nil {
		t.Fatal(err)
	}
	want := []Config{
		{
			Host:            "include1.local",
			Listen:          ":8080",
			Server:          Server{Name: "fasthttpd"},
			SSL:             SSL{}.SetDefaults(),
			Log:             Log{}.SetDefaults(),
			AccessLog:       AccessLog{}.SetDefaults(),
			ShutdownTimeout: DefaultShutdownTimeout,
		}, {
			Host:            "include2.local",
			Listen:          ":8080",
			Server:          Server{Name: "fasthttpd"},
			SSL:             SSL{}.SetDefaults(),
			Log:             Log{}.SetDefaults(),
			AccessLog:       AccessLog{}.SetDefaults(),
			ShutdownTimeout: DefaultShutdownTimeout,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v\nwant %#v", got, want)
	}
}

func TestUnmarshalYAMLPath_IncludeCircular(t *testing.T) {
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(currentDir) //nolint:errcheck

	if err := os.Chdir("testdata"); err != nil {
		t.Fatal(err)
	}

	_, err = UnmarshalYAMLPath("include-circular.yaml")
	if err == nil {
		t.Fatalf("no error")
	}
	if err.Error() != `circular dependency [include-circular.yaml]` {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestConfig_Normalize(t *testing.T) {
	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		cfg    Config
		want   Config
		errstr string
	}{
		{
			cfg:  Config{},
			want: Config{},
		}, {
			// Normalize no longer touches Server fields: duration
			// parsing happens inside Server.Duration.UnmarshalYAML
			// at decode time. Kept here so the invariant "an empty
			// Config normalizes to itself" stays covered.
			cfg: Config{
				Server: Server{ReadTimeout: Duration(60 * time.Second)},
			},
			want: Config{
				Server: Server{ReadTimeout: Duration(60 * time.Second)},
			},
		}, {
			cfg: Config{
				ShutdownTimeout: "invalid duration",
			},
			errstr: `failed to parse shutdownTimeout: time: invalid duration "invalid duration"`,
		}, {
			cfg: Config{
				SSL: SSL{
					AutoCert: true,
				},
			},
			want: Config{
				SSL: SSL{
					AutoCert:         true,
					AutoCertCacheDir: filepath.Join(userCacheDir, "fasthttpd", "cert"),
				},
			},
		}, {
			cfg: Config{
				Handlers: map[string]tree.Map{},
				Routes: []Route{
					{
						Handler: "",
					}, {
						Handler: "UNKNOWN",
					},
				},
			},
			errstr: `unknown handler "UNKNOWN"`,
		},
	}
	for i, test := range tests {
		got, err := test.cfg.Normalize()
		if test.errstr != "" {
			if err == nil {
				t.Fatalf("tests[%d] no error", i)
			}
			if !strings.HasPrefix(err.Error(), test.errstr) {
				t.Errorf("tests[%d] got %q; want %q", i, err.Error(), test.errstr)
			}
			continue
		}
		if err != nil {
			t.Fatalf("tests[%d] error %v", i, err)
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("tests[%d] got %#v; want %#v", i, got, test.want)
		}
	}
}

func TestParseSize(t *testing.T) {
	testCases := []struct {
		caseName string
		input    string
		want     int64
		wantErr  bool
	}{
		{caseName: "bare integer", input: "4096", want: 4096},
		{caseName: "K suffix", input: "4K", want: 4 << 10},
		{caseName: "lowercase k", input: "4k", want: 4 << 10},
		{caseName: "KB suffix", input: "4KB", want: 4 << 10},
		{caseName: "KiB suffix", input: "4KiB", want: 4 << 10},
		{caseName: "space before suffix", input: "4 kB", want: 4 << 10},
		{caseName: "M suffix", input: "2M", want: 2 << 20},
		{caseName: "G suffix", input: "1G", want: 1 << 30},
		{caseName: "zero", input: "0", want: 0},
		{caseName: "negative rejected", input: "-4K", wantErr: true},
		{caseName: "decimal rejected", input: "1.5M", wantErr: true},
		{caseName: "unknown suffix", input: "4T", wantErr: true},
		{caseName: "empty", input: "", wantErr: true},
		{caseName: "letters only", input: "abc", wantErr: true},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			got, err := parseSize(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseSize(%q) returned %d, want error", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSize(%q) returned %v, want %d", tc.input, err, tc.want)
			}
			if got != tc.want {
				t.Errorf("parseSize(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

func TestSize_UnmarshalYAML(t *testing.T) {
	testCases := []struct {
		caseName string
		yaml     string
		want     Size
		wantErr  bool
	}{
		{caseName: "integer", yaml: "4096", want: 4096},
		{caseName: "string with K", yaml: `"4k"`, want: 4096},
		{caseName: "string with M", yaml: `"2M"`, want: 2 << 20},
		{caseName: "invalid string", yaml: `"abc"`, wantErr: true},
		{caseName: "boolean rejected", yaml: "true", wantErr: true},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			var got Size
			err := yaml.Unmarshal([]byte(tc.yaml), &got)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("yaml unmarshal %q returned %d, want error", tc.yaml, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("yaml unmarshal %q returned %v, want %d", tc.yaml, err, tc.want)
			}
			if got != tc.want {
				t.Errorf("yaml unmarshal %q = %d, want %d", tc.yaml, got, tc.want)
			}
		})
	}
}

func TestDuration_UnmarshalYAML(t *testing.T) {
	testCases := []struct {
		caseName string
		yaml     string
		want     Duration
		wantErr  bool
	}{
		{caseName: "string duration", yaml: `"60s"`, want: Duration(60 * time.Second)},
		{caseName: "compound string", yaml: `"1h30m"`, want: Duration(90 * time.Minute)},
		{caseName: "integer nanoseconds", yaml: "1000000000", want: Duration(time.Second)},
		{caseName: "invalid string", yaml: `"not-a-duration"`, wantErr: true},
		{caseName: "boolean rejected", yaml: "true", wantErr: true},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			var got Duration
			err := yaml.Unmarshal([]byte(tc.yaml), &got)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("yaml unmarshal %q returned %v, want error", tc.yaml, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("yaml unmarshal %q returned %v, want %v", tc.yaml, err, tc.want)
			}
			if got != tc.want {
				t.Errorf("yaml unmarshal %q = %v, want %v", tc.yaml, got, tc.want)
			}
		})
	}
}

func TestServer_ApplyTo(t *testing.T) {
	src := Server{
		Name:                          "custom",
		Concurrency:                   42,
		ReadBufferSize:                4096,
		WriteBufferSize:               8192,
		MaxConnsPerIP:                 10,
		MaxRequestsPerConn:            100,
		MaxRequestBodySize:            1 << 20,
		ReadTimeout:                   Duration(60 * time.Second),
		WriteTimeout:                  Duration(30 * time.Second),
		IdleTimeout:                   Duration(15 * time.Second),
		MaxIdleWorkerDuration:         Duration(5 * time.Second),
		TCPKeepalivePeriod:            Duration(10 * time.Second),
		DisableKeepalive:              true,
		TCPKeepalive:                  true,
		ReduceMemoryUsage:             true,
		NoDefaultServerHeader:         true,
		NoDefaultDate:                 true,
		NoDefaultContentType:          true,
		DisableHeaderNamesNormalizing: true,
		LogAllErrors:                  true,
		SecureErrorLogMessage:         true,
		DisablePreParseMultipartForm:  true,
		CloseOnShutdown:               true,
		StreamRequestBody:             true,
	}
	var dst fasthttp.Server
	src.ApplyTo(&dst)

	if dst.Name != "custom" {
		t.Errorf("Name = %q, want %q", dst.Name, "custom")
	}
	if dst.Concurrency != 42 {
		t.Errorf("Concurrency = %d, want 42", dst.Concurrency)
	}
	if dst.ReadBufferSize != 4096 {
		t.Errorf("ReadBufferSize = %d, want 4096", dst.ReadBufferSize)
	}
	if dst.MaxRequestBodySize != 1<<20 {
		t.Errorf("MaxRequestBodySize = %d, want %d", dst.MaxRequestBodySize, 1<<20)
	}
	if dst.ReadTimeout != 60*time.Second {
		t.Errorf("ReadTimeout = %v, want 60s", dst.ReadTimeout)
	}
	if !dst.DisableKeepalive {
		t.Error("DisableKeepalive not propagated")
	}
	if !dst.StreamRequestBody {
		t.Error("StreamRequestBody not propagated")
	}
}
