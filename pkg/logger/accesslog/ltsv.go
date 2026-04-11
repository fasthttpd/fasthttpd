package accesslog

import (
	"net"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/valyala/fasthttp"
)

// FormatLTSV is the keyword that selects the preset LTSV output format.
const FormatLTSV = "ltsv"

// newLTSVAccessLog builds an *accessLog that writes records in the preset
// LTSV format defined by appendLTSVLog.
func newLTSVAccessLog(out logger.Rotator, cfg config.Config) (*accessLog, error) {
	l := newSkeleton(out, cfg)
	l.appendLine = appendLTSVLog
	l.startFlushLoop(cfg)
	return l, nil
}

// appendLTSVValue appends s to dst, replacing any TAB/LF/CR byte with a
// single space so the result is safe as an LTSV field value. It is
// allocation-free as long as dst has sufficient capacity.
func appendLTSVValue(dst, s []byte) []byte {
	for i := range len(s) {
		c := s[i]
		switch c {
		case '\t', '\n', '\r':
			dst = append(dst, ' ')
		default:
			dst = append(dst, c)
		}
	}
	return dst
}

// appendHostIP appends the IP portion of addr to dst without port or
// brackets, matching nginx's $remote_addr. It is allocation-free for the
// common *net.TCPAddr case.
func appendHostIP(dst []byte, addr net.Addr) []byte {
	if ta, ok := addr.(*net.TCPAddr); ok {
		if ip4 := ta.IP.To4(); ip4 != nil {
			return fasthttp.AppendIPv4(dst, ip4)
		}
		return append(dst, ta.IP.String()...)
	}
	return dst
}

// appendLTSVLog writes one access log record to dst using the preset LTSV
// schema. It is allocation-free for the common TCP path.
//
// The field list follows nginx/fluentd LTSV conventions:
//
//	time host forwardedfor user req scheme vhost status size
//	reqsize reqtime_microsec referer ua
//
// host is the direct peer IP (without port), matching nginx's $remote_addr.
// forwardedfor is the raw X-Forwarded-For header value. req combines method,
// URI and protocol NCSA-style. Empty string fields are written as empty
// values, not "-".
func appendLTSVLog(dst []byte, ctx *fasthttp.RequestCtx) []byte {
	dst = append(dst, "time:"...)
	dst = appendISOTime(dst, ctx.Time())

	dst = append(dst, "\thost:"...)
	dst = appendHostIP(dst, ctx.RemoteAddr())

	dst = append(dst, "\tforwardedfor:"...)
	dst = appendLTSVValue(dst, ctx.Request.Header.PeekBytes(xForwardedForKey))

	dst = append(dst, "\tuser:"...)
	dst = appendLTSVValue(dst, ctx.Request.URI().Username())

	dst = append(dst, "\treq:"...)
	dst = appendLTSVValue(dst, ctx.Method())
	dst = append(dst, ' ')
	dst = appendLTSVValue(dst, ctx.RequestURI())
	dst = append(dst, ' ')
	dst = appendLTSVValue(dst, ctx.Request.Header.Protocol())

	dst = append(dst, "\tscheme:"...)
	dst = appendLTSVValue(dst, ctx.URI().Scheme())

	dst = append(dst, "\tvhost:"...)
	dst = appendLTSVValue(dst, ctx.Host())

	dst = append(dst, "\tstatus:"...)
	dst = fasthttp.AppendUint(dst, ctx.Response.StatusCode())

	dst = append(dst, "\tsize:"...)
	dst = fasthttp.AppendUint(dst, ctx.Response.Header.ContentLength())

	dst = append(dst, "\treqsize:"...)
	bytesIn := len(ctx.Request.Header.RawHeaders()) + len(ctx.Request.Body())
	dst = fasthttp.AppendUint(dst, bytesIn)

	dst = append(dst, "\treqtime_microsec:"...)
	durationUs := int(timeNow().Sub(ctx.Time()).Microseconds())
	dst = fasthttp.AppendUint(dst, durationUs)

	dst = append(dst, "\treferer:"...)
	dst = appendLTSVValue(dst, ctx.Request.Header.Referer())

	dst = append(dst, "\tua:"...)
	dst = appendLTSVValue(dst, ctx.Request.Header.UserAgent())

	return dst
}
