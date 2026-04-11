package accesslog

import (
	"net"
	"os"
	"regexp"
	"time"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/fasthttpd/fasthttpd/pkg/util"
	"github.com/valyala/fasthttp"
)

const (
	FormatCommon   = `%h %l %u %t "%r" %>s %b`
	FormatCombined = `%h %l %u %t "%r" %>s %b "%{Referer}i" "%{User-agent}i"`
)

var formatPattern = regexp.MustCompile(`(%(>|{(.+?)})?([a-zA-Z%])|([^%]+))`)

// newNCSAAccessLog builds an *accessLog that writes records in the Apache-style
// NCSA format specified by cfg.AccessLog.Format (or FormatCommon when empty).
func newNCSAAccessLog(out logger.Rotator, cfg config.Config) (*accessLog, error) {
	l := newSkeleton(out, cfg)

	format := cfg.AccessLog.Format
	if format == "" {
		format = FormatCommon
	}

	fns, err := l.parseNCSAFormat(format, cfg)
	if err != nil {
		return nil, err
	}
	l.appendLine = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		for _, fn := range fns {
			dst = fn(dst, ctx)
		}
		return dst
	}

	l.startFlushLoop(cfg)
	return l, nil
}

func (l *accessLog) parseNCSAFormat(format string, cfg config.Config) ([]appendFunc, error) {
	var fns []appendFunc
	for _, ms := range formatPattern.FindAllStringSubmatch(format, -1) {
		k := ms[4]
		switch k {
		case "":
			fns = append(fns, newAppendBytes([]byte(ms[0])))
		case "C":
			fns = append(fns, newAppendCookie(ms[3]))
		case "e":
			fns = append(fns, newAppendEnv(ms[3]))
		case "i":
			fns = append(fns, newAppendRequestHeader(ms[3]))
		case "o":
			fns = append(fns, newAppendResponseHeader(ms[3]))
		case "p":
			if ms[3] == "remote" {
				fns = append(fns, appendLpRemote)
			} else {
				fns = append(fns, appendLp)
			}
		case "t":
			if ms[3] == "" {
				fns = append(fns, appendLt)
			} else {
				fns = append(fns, newAppendStrftime(ms[3]))
			}
		case "v":
			if cfg.Host != "" {
				fns = append(fns, newAppendBytes([]byte(cfg.Host)))
			} else {
				fns = append(fns, appendNil)
			}
		case "V":
			canonicalHost, err := os.Hostname()
			if err != nil {
				return nil, err
			}
			fns = append(fns, newAppendBytes([]byte(canonicalHost)))
		case "r":
			l.collectRequestURI = true
			fallthrough
		default:
			if fn, ok := appendFuncs[k]; ok {
				fns = append(fns, fn)
			} else {
				fns = append(fns, newAppendBytes([]byte("%"+k)))
			}
		}
	}
	return fns, nil
}

// appendNetAddr appends the string representation of addr to dst without
// allocating for the common *net.TCPAddr case.
func appendNetAddr(dst []byte, addr net.Addr) []byte {
	if ta, ok := addr.(*net.TCPAddr); ok {
		if ip4 := ta.IP.To4(); ip4 != nil {
			dst = fasthttp.AppendIPv4(dst, ip4)
		} else {
			dst = append(dst, '[')
			dst = append(dst, ta.IP.String()...)
			dst = append(dst, ']')
		}
		dst = append(dst, ':')
		dst = fasthttp.AppendUint(dst, ta.Port)
		return dst
	}
	return append(dst, addr.String()...)
}

// portFromAddr extracts the port from addr without allocating for *net.TCPAddr.
func portFromAddr(addr net.Addr) int {
	if ta, ok := addr.(*net.TCPAddr); ok {
		return ta.Port
	}
	return 0
}

func appendLp(dst []byte, ctx *fasthttp.RequestCtx) []byte {
	if p := portFromAddr(ctx.LocalAddr()); p > 0 {
		return fasthttp.AppendUint(dst, p)
	}
	return appendNil(dst, nil)
}

func appendLpRemote(dst []byte, ctx *fasthttp.RequestCtx) []byte {
	if p := portFromAddr(ctx.RemoteAddr()); p > 0 {
		return fasthttp.AppendUint(dst, p)
	}
	return appendNil(dst, nil)
}

var ncsaMonths = []byte("JanFebMarAprMayJunJulAugSepOctNovDec")

// appendNCSADate appends NCSA common format of date (dd/MMM/yyyy:hh:mm:ss +-hhmm)
// to dst and returns the extended dst.
func appendNCSADate(dst []byte, date time.Time) []byte {
	m := (date.Month() - 1) * 3
	_, zo := date.Zone()

	b := []byte{
		'[',
		'0', 0,
		'/',
		0, 0, 0,
		'/',
		'0', '0', '0', 0,
		':',
		'0', 0,
		':',
		'0', 0,
		':',
		'0', 0,
		' ',
		0, '0', 0,
		'0', '0', ']',
	}
	util.CopyRightUint(b[1:3], date.Day())
	copy(b[4:7], ncsaMonths[m:m+3])
	util.CopyRightUint(b[8:12], date.Year())
	util.CopyRightUint(b[13:15], date.Hour())
	util.CopyRightUint(b[16:18], date.Minute())
	util.CopyRightUint(b[19:21], date.Second())
	if zo < 0 {
		b[22] = '-'
		zo = -zo
	} else {
		b[22] = '+'
	}
	util.CopyRightUint(b[23:25], zo/(60*60))

	return append(dst, b...)
}

// appendNCSARequest appends NCSA common format of request (method uri protocol)
// to dst and returns the extended dst.
func appendNCSARequest(dst, method, uri, protocol []byte) []byte {
	dst = append(dst, method...)
	dst = append(dst, ' ')
	for _, s := range uri {
		switch s {
		case '"', '\\':
			dst = append(dst, '\\')
		}
		dst = append(dst, s)
	}
	dst = append(dst, ' ')
	dst = append(dst, protocol...)
	return dst
}

func newAppendBytes(b []byte) appendFunc {
	return func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		return append(dst, b...)
	}
}

func newAppendCookie(key string) appendFunc {
	bytesKey := []byte(key)
	return func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		if v := ctx.Request.Header.CookieBytes(bytesKey); len(v) > 0 {
			return append(dst, v...)
		}
		return appendNil(dst, nil)
	}
}

func newAppendEnv(key string) appendFunc {
	return func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		if v, ok := os.LookupEnv(key); ok {
			return append(dst, v...)
		}
		return appendNil(dst, nil)
	}
}

func newAppendRequestHeader(key string) appendFunc {
	bytesKey := []byte(key)
	return func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		if v := ctx.Request.Header.PeekBytes(bytesKey); len(v) > 0 {
			return append(dst, v...)
		}
		return appendNil(dst, nil)
	}
}

func newAppendResponseHeader(key string) appendFunc {
	bytesKey := []byte(key)
	return func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		if v := ctx.Response.Header.PeekBytes(bytesKey); len(v) > 0 {
			return append(dst, v...)
		}
		return appendNil(dst, nil)
	}
}

func newAppendStrftime(format string) appendFunc {
	s := util.NewStrftime(format)
	return func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		return s.AppendBytes(dst, ctx.Time())
	}
}

var (
	appendNil  = newAppendBytes([]byte{'-'})
	appendPlus = newAppendBytes([]byte{'+'})
	appendLa   = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		return appendNetAddr(dst, ctx.RemoteAddr())
	}
	appendA = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		return appendNetAddr(dst, ctx.LocalAddr())
	}
	appendB = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		return fasthttp.AppendUint(dst, ctx.Response.Header.ContentLength())
	}
	appendLb = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		if b := ctx.Response.Header.ContentLength(); b > 0 {
			return fasthttp.AppendUint(dst, b)
		}
		return appendNil(dst, nil)
	}
	appendD = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		t := timeNow().Sub(ctx.Time())
		return fasthttp.AppendUint(dst, int(t.Microseconds()))
	}
	appendLf = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		return append(dst, ctx.Path()...)
	}
	appendLh = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		return fasthttp.AppendIPv4(dst, ctx.RemoteIP())
	}
	appendLk = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		return fasthttp.AppendUint(dst, int(ctx.ConnRequestNum()))
	}
	appendH = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		return append(dst, ctx.Request.Header.Protocol()...)
	}
	appendLm = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		return append(dst, ctx.Method()...)
	}
	appendP = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		return fasthttp.AppendUint(dst, os.Getegid())
	}
	appendLq = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		if qstr := ctx.URI().QueryString(); len(qstr) > 0 {
			return append(append(dst, '?'), qstr...)
		}
		return appendNil(dst, nil)
	}
	appendLr = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		uri := ctx.Request.Header.PeekBytes(HeaderKeyOriginalRequestURI)
		if len(uri) == 0 {
			return appendNil(dst, nil)
		}
		return appendNCSARequest(dst, ctx.Method(), uri, ctx.Request.Header.Protocol())
	}
	appendL = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		return fasthttp.AppendUint(dst, int(ctx.ID()))
	}
	appendLs = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		return fasthttp.AppendUint(dst, ctx.Response.StatusCode())
	}
	appendLt = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		return appendNCSADate(dst, ctx.Time())
	}
	appendT = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		t := timeNow().Sub(ctx.Time())
		return fasthttp.AppendUint(dst, int(t.Seconds()))
	}
	appendLu = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		if u := ctx.Request.URI().Username(); len(u) > 0 {
			return append(dst, u...)
		}
		return appendNil(dst, nil)
	}
	appendU = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		return append(dst, ctx.Path()...)
	}
	appendX = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		if !ctx.Response.ConnectionClose() {
			return appendPlus(dst, nil)
		}
		return appendNil(dst, nil)
	}
	appendI = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		h := len(ctx.Request.Header.RawHeaders())
		b := len(ctx.Request.Body())
		return fasthttp.AppendUint(dst, h+b)
	}
	appendO = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		h := len(ctx.Response.Header.Header())
		b := len(ctx.Response.Body())
		return fasthttp.AppendUint(dst, h+b)
	}
	appendS = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		h := len(ctx.Request.Header.RawHeaders())
		b := len(ctx.Request.Body())
		h += len(ctx.Response.Header.Header())
		b += len(ctx.Response.Body())
		return fasthttp.AppendUint(dst, h+b)
	}
)

// Refer to https://httpd.apache.org/docs/2.4/en/mod/mod_log_config.html
var appendFuncs = map[string]appendFunc{
	"%": newAppendBytes([]byte{'%'}),
	"a": appendLa,
	"A": appendA,
	"B": appendB,
	"b": appendLb,
	"D": appendD,
	"f": appendLf,
	"h": appendLh,
	"H": appendH,
	"k": appendLk,
	"l": appendNil, // Unsupported
	"L": appendL,
	"m": appendLm,
	"P": appendP,
	"q": appendLq,
	"r": appendLr,
	"R": appendNil, // Unsupported
	"s": appendLs,
	"t": appendLt,
	"T": appendT,
	"u": appendLu,
	"U": appendU,
	"X": appendX,
	"I": appendI,
	"O": appendO,
	"S": appendS,
}
