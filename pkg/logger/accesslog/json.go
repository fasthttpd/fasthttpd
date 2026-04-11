package accesslog

import (
	"net"
	"time"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/valyala/fasthttp"
)

// FormatJSON is the keyword that selects the preset JSON output format.
const FormatJSON = "json"

// newJSONAccessLog builds an *accessLog that writes records in the preset
// JSON format defined by appendJSONLog.
func newJSONAccessLog(out logger.Rotator, cfg config.Config) (*accessLog, error) {
	l := newSkeleton(out, cfg)
	l.appendLine = appendJSONLog
	l.startFlushLoop(cfg)
	return l, nil
}

var xForwardedForKey = []byte("X-Forwarded-For")

const hexDigits = "0123456789abcdef"

// appendJSONString appends s to dst as a JSON string literal, including the
// surrounding quotes and any required escaping. It is allocation-free as long
// as dst has sufficient capacity.
func appendJSONString(dst, s []byte) []byte {
	dst = append(dst, '"')
	for i := range len(s) {
		c := s[i]
		switch c {
		case '"':
			dst = append(dst, '\\', '"')
		case '\\':
			dst = append(dst, '\\', '\\')
		case '\b':
			dst = append(dst, '\\', 'b')
		case '\t':
			dst = append(dst, '\\', 't')
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\f':
			dst = append(dst, '\\', 'f')
		case '\r':
			dst = append(dst, '\\', 'r')
		default:
			if c < 0x20 {
				dst = append(dst, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0x0f])
			} else {
				dst = append(dst, c)
			}
		}
	}
	dst = append(dst, '"')
	return dst
}

// appendISOTime appends t to dst as an RFC 3339 timestamp using the local
// timezone offset (e.g. "2026-04-11T10:19:13+09:00"). It is allocation-free.
func appendISOTime(dst []byte, t time.Time) []byte {
	year, month, day := t.Date()
	hour, minute, second := t.Clock()

	dst = appendUint4(dst, year)
	dst = append(dst, '-')
	dst = appendUint2(dst, int(month))
	dst = append(dst, '-')
	dst = appendUint2(dst, day)
	dst = append(dst, 'T')
	dst = appendUint2(dst, hour)
	dst = append(dst, ':')
	dst = appendUint2(dst, minute)
	dst = append(dst, ':')
	dst = appendUint2(dst, second)

	_, offsetSec := t.Zone()
	if offsetSec == 0 {
		return append(dst, 'Z')
	}
	if offsetSec < 0 {
		dst = append(dst, '-')
		offsetSec = -offsetSec
	} else {
		dst = append(dst, '+')
	}
	dst = appendUint2(dst, offsetSec/3600)
	dst = append(dst, ':')
	dst = appendUint2(dst, (offsetSec%3600)/60)
	return dst
}

func appendUint2(dst []byte, n int) []byte {
	return append(dst, byte('0'+n/10), byte('0'+n%10))
}

func appendUint4(dst []byte, n int) []byte {
	return append(dst,
		byte('0'+n/1000),
		byte('0'+(n/100)%10),
		byte('0'+(n/10)%10),
		byte('0'+n%10),
	)
}

// appendJSONClientIP appends the client IP, derived from the left-most entry
// of X-Forwarded-For when present, or the IP portion of RemoteAddr otherwise.
// The result is wrapped in JSON quotes.
func appendJSONClientIP(dst []byte, ctx *fasthttp.RequestCtx) []byte {
	dst = append(dst, '"')
	if xff := ctx.Request.Header.PeekBytes(xForwardedForKey); len(xff) > 0 {
		end := len(xff)
		for i := range len(xff) {
			if xff[i] == ',' {
				end = i
				break
			}
		}
		start := 0
		for start < end && (xff[start] == ' ' || xff[start] == '\t') {
			start++
		}
		for end > start && (xff[end-1] == ' ' || xff[end-1] == '\t') {
			end--
		}
		// X-Forwarded-For values are IPs and contain no JSON-special chars.
		dst = append(dst, xff[start:end]...)
	} else if ta, ok := ctx.RemoteAddr().(*net.TCPAddr); ok {
		if ip4 := ta.IP.To4(); ip4 != nil {
			dst = fasthttp.AppendIPv4(dst, ip4)
		} else {
			dst = append(dst, ta.IP.String()...)
		}
	}
	dst = append(dst, '"')
	return dst
}

// appendJSONLog writes one access log record to dst as a JSON object using
// the preset 15-field schema. It is allocation-free for the common TCP path.
func appendJSONLog(dst []byte, ctx *fasthttp.RequestCtx) []byte {
	dst = append(dst, `{"time":"`...)
	dst = appendISOTime(dst, ctx.Time())

	dst = append(dst, `","remote_addr":"`...)
	dst = appendNetAddr(dst, ctx.RemoteAddr())
	dst = append(dst, '"')

	dst = append(dst, `,"client_ip":`...)
	dst = appendJSONClientIP(dst, ctx)

	dst = append(dst, `,"remote_user":`...)
	dst = appendJSONString(dst, ctx.Request.URI().Username())

	dst = append(dst, `,"method":`...)
	dst = appendJSONString(dst, ctx.Method())

	dst = append(dst, `,"uri":`...)
	dst = appendJSONString(dst, ctx.RequestURI())

	dst = append(dst, `,"proto":`...)
	dst = appendJSONString(dst, ctx.Request.Header.Protocol())

	dst = append(dst, `,"scheme":`...)
	dst = appendJSONString(dst, ctx.URI().Scheme())

	dst = append(dst, `,"host":`...)
	dst = appendJSONString(dst, ctx.Host())

	dst = append(dst, `,"status":`...)
	dst = fasthttp.AppendUint(dst, ctx.Response.StatusCode())

	dst = append(dst, `,"size":`...)
	dst = fasthttp.AppendUint(dst, ctx.Response.Header.ContentLength())

	dst = append(dst, `,"bytes_received":`...)
	bytesIn := len(ctx.Request.Header.RawHeaders()) + len(ctx.Request.Body())
	dst = fasthttp.AppendUint(dst, bytesIn)

	dst = append(dst, `,"duration_us":`...)
	durationUs := int(timeNow().Sub(ctx.Time()).Microseconds())
	dst = fasthttp.AppendUint(dst, durationUs)

	dst = append(dst, `,"referer":`...)
	dst = appendJSONString(dst, ctx.Request.Header.Referer())

	dst = append(dst, `,"user_agent":`...)
	dst = appendJSONString(dst, ctx.Request.Header.UserAgent())

	dst = append(dst, '}')
	return dst
}
