package accesslog

import (
	"io"
	"net"
	"os"
	"regexp"
	"time"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/fasthttpd/fasthttpd/pkg/util"
	"github.com/valyala/bytebufferpool"
	"github.com/valyala/fasthttp"
)

const (
	FormatCommon   = `%h %l %u %t "%r" %>s %b`
	FormatCombined = `%h %l %u %t "%r" %>s %b "%{Referer}i" "%{User-agent}i"`
)

var (
	// UserKeyOriginalRequestURI is a key to store original RequestURI.
	UserKeyOriginalRequestURI = []byte("Original-Request-URI")
	// UserKeyUsername is a key to store username.
	UserKeyUsername = []byte("Username")
)

// AccessLog is an interface to write access log.
type AccessLog interface {
	logger.Rotator
	io.Closer
	// Collect collects informations before Log.
	Collect(ctx *fasthttp.RequestCtx)
	// Log writes access log.
	Log(ctx *fasthttp.RequestCtx)
}

// NewAccessLog returns a new AccessLog.
func NewAccessLog(cfg config.Config) (AccessLog, error) {
	out, err := logger.SharedRotator(cfg.AccessLog.Output, cfg.AccessLog.Rotation)
	if err != nil {
		return nil, err
	}
	l, err := newAccessLog(out, cfg)
	if err != nil {
		out.Close() //nolint:errcheck
		return nil, err
	}
	return l, nil
}

type accessLog struct {
	out               logger.Rotator
	appendFuncs       []appendFunc
	collectRequestURI bool
	addrToPortCache   util.Cache
}

func newAccessLog(out logger.Rotator, cfg config.Config) (*accessLog, error) {
	return (&accessLog{out: out}).init(cfg)
}

var formatPattern = regexp.MustCompile(`(%(>|{(.+?)})?([a-zA-Z%])|([^%]+))`)

var timeNow = func() time.Time { return time.Now() }

// Rotate rotate log stream.
func (l *accessLog) Rotate() error {
	return l.out.Rotate()
}

// Write writes to log stream.
func (l *accessLog) Write(p []byte) (int, error) {
	return l.out.Write(p)
}

// Close closes log stream.
func (l *accessLog) Close() error {
	l.appendFuncs = nil
	l.collectRequestURI = false
	l.addrToPortCache = nil
	if out := l.out; out != nil {
		l.out = logger.NilRotator
		if err := out.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (l *accessLog) init(cfg config.Config) (*accessLog, error) {
	format := cfg.AccessLog.Format
	if format == "" {
		format = FormatCommon
	}

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
				fns = append(fns, l.appendLpRemote)
			} else {
				fns = append(fns, l.appendLp)
			}
			if l.addrToPortCache == nil {
				l.addrToPortCache = util.NewExpireCache(0)
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
	l.appendFuncs = fns

	return l, nil
}

// Collect stores Request-URI to ctx as UserValue if '%r' is specified in format.
func (l *accessLog) Collect(ctx *fasthttp.RequestCtx) {
	if l.collectRequestURI {
		// NOTE: store string
		uri := string(ctx.URI().RequestURI())
		ctx.SetUserValueBytes(UserKeyOriginalRequestURI, uri)
	}
}

func (l *accessLog) Log(ctx *fasthttp.RequestCtx) {
	b := bytebufferpool.Get()
	for _, fn := range l.appendFuncs {
		b.B = fn(b.B, ctx)
	}
	b.B = append(b.B, '\n')

	go func() {
		defer bytebufferpool.Put(b)
		l.out.Write(b.B) //nolint:errcheck
	}()
}

func (l *accessLog) portFromAddr(addr string) []byte {
	key := util.CacheKeyString(addr)
	if portBytes := l.addrToPortCache.Get(key); portBytes != nil {
		return portBytes.([]byte)
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "0" {
		l.addrToPortCache.Set(key, []byte{})
		return nil
	}
	portBytes := []byte(port)
	l.addrToPortCache.Set(key, portBytes)
	return portBytes
}

func (l *accessLog) appendLp(dst []byte, ctx *fasthttp.RequestCtx) []byte {
	if p := l.portFromAddr(ctx.LocalAddr().String()); len(p) > 0 {
		return append(dst, p...)
	}
	return appendNil(dst, nil)
}

func (l *accessLog) appendLpRemote(dst []byte, ctx *fasthttp.RequestCtx) []byte {
	if p := l.portFromAddr(ctx.RemoteAddr().String()); len(p) > 0 {
		return append(dst, p...)
	}
	return appendNil(dst, nil)
}

var ncsaMonths = []byte("JanFebMarAprMayJunJulAugSepOctNovDec")

// appendNCSADate appends NCSA common format of date (dd/MMM/yyyy:hh:mm:ss +-hhmm)
// to dst and returns the extended dst.
func appendNCSADate(dst []byte, date time.Time) []byte {
	m := (date.Month() - 1) * 3
	_, zo := date.Zone()

	dst = append(dst, '[')
	dst = util.AppendZeroPaddingUint(dst, date.Day(), 2)
	dst = append(dst, '/')
	dst = append(dst, ncsaMonths[m:m+3]...)
	dst = append(dst, '/')
	dst = util.AppendZeroPaddingUint(dst, date.Year(), 4)
	dst = append(dst, ':')
	dst = util.AppendZeroPaddingUint(dst, date.Hour(), 2)
	dst = append(dst, ':')
	dst = util.AppendZeroPaddingUint(dst, date.Minute(), 2)
	dst = append(dst, ':')
	dst = util.AppendZeroPaddingUint(dst, date.Second(), 2)
	dst = append(dst, ' ')
	if zo < 0 {
		dst = append(dst, '-')
		zo = -zo
	} else {
		dst = append(dst, '+')
	}
	dst = util.AppendZeroPaddingUint(dst, zo/(60*60), 2)
	dst = append(dst, '0', '0', ']')

	return dst
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

type appendFunc func(dst []byte, ctx *fasthttp.RequestCtx) []byte

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
	// appendNil appends "-".
	appendNil = newAppendBytes([]byte{'-'})
	// appendNil appends "+".
	appendPlus = newAppendBytes([]byte{'+'})
	// appendLa appends client IP address of the request.
	appendLa = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		return append(dst, []byte(ctx.RemoteAddr().String())...)
	}
	// appendA appends underlying peer IP address of the connection.
	appendA = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		return append(dst, []byte(ctx.LocalAddr().String())...)
	}
	// appendB appends size of response in bytes, excluding HTTP headers.
	appendB = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		return fasthttp.AppendUint(dst, ctx.Response.Header.ContentLength())
	}
	// appendLb appends size of response in bytes, excluding HTTP headers.
	// In CLF format, i.e. a '-' rather than a 0 when no bytes are sent.
	appendLb = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		if b := ctx.Response.Header.ContentLength(); b > 0 {
			return fasthttp.AppendUint(dst, b)
		}
		return appendNil(dst, nil)
	}
	// appendD appends the time taken to serve the request, in microseconds.
	appendD = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		t := timeNow().Sub(ctx.Time())
		return fasthttp.AppendUint(dst, int(t.Microseconds()))
	}
	// appendLf appends filename.
	appendLf = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		// TODO: not absolute path
		return append(dst, ctx.Path()...)
	}
	// appendLh appends remote ip.
	appendLh = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		return fasthttp.AppendIPv4(dst, ctx.RemoteIP())
	}
	// appendLk appends number of keepalive requests handled on this connection.
	appendLk = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		return fasthttp.AppendUint(dst, int(ctx.ConnRequestNum()))
	}
	// appendH appends the request protocol.
	appendH = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		return append(dst, ctx.Request.Header.Protocol()...)
	}
	// appendLm appends the request method.
	appendLm = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		return append(dst, ctx.Method()...)
	}
	// appendP appends the process ID of the child that serviced the request.
	appendP = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		return fasthttp.AppendUint(dst, os.Getegid())
	}
	// appendLq appends the query string (prepended with a ? if a query string
	// exists, otherwise an empty string).
	appendLq = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		if qstr := ctx.URI().QueryString(); len(qstr) > 0 {
			return append(append(dst, '?'), qstr...)
		}
		return appendNil(dst, nil)
	}
	// appendLr appends first line of request.
	appendLr = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		uri := ctx.UserValueBytes(UserKeyOriginalRequestURI).(string)
		return appendNCSARequest(dst, ctx.Method(), []byte(uri), ctx.Request.Header.Protocol())
	}
	// appendL appends the request log ID from the error log (or '-' if
	// nothing has been logged to the error log for this request).
	appendL = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		return fasthttp.AppendUint(dst, int(ctx.ID()))
	}
	// appendLs appends final status. %s and %>s are mapped this function.
	appendLs = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		return fasthttp.AppendUint(dst, ctx.Response.StatusCode())
	}
	// appendLt appends time the request was received, in the format
	// [18/Sep/2011:19:18:28 -0400].
	appendLt = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		return appendNCSADate(dst, ctx.Time())
	}
	// appendT appends the time taken to serve the request, in seconds.
	appendT = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		t := timeNow().Sub(ctx.Time())
		return fasthttp.AppendUint(dst, int(t.Seconds()))
	}
	// appendLu appends remote user if the request was authenticated.
	appendLu = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		if u := ctx.Request.URI().Username(); len(u) > 0 {
			return append(dst, u...)
		}
		return appendNil(dst, nil)
	}
	// appendU appends the URL path requested, not including any query string.
	appendU = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		return append(dst, ctx.Path()...)
	}
	// appendX appends connection status when response is completed:
	//   X = Connection aborted before the response completed.
	//   + = Connection may be kept alive after the response is sent.
	//   - = Connection will be closed after the response is sent.
	appendX = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		// TODO: unsupported X
		if !ctx.Response.ConnectionClose() {
			return appendPlus(dst, nil)
		}
		return appendNil(dst, nil)
	}
	// appendI appends bytes received, including request and headers.
	// Cannot be zero.
	appendI = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		h := len(ctx.Request.Header.RawHeaders())
		b := len(ctx.Request.Body())
		return fasthttp.AppendUint(dst, h+b)
	}
	// appendO appends bytes sent, including headers. May be zero in rare cases
	// such as when a request is aborted before a response is sent.
	appendO = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		h := len(ctx.Response.Header.Header())
		b := len(ctx.Response.Body())
		return fasthttp.AppendUint(dst, h+b)
	}
	// appendS appends bytes transferred (received and sent), including request
	//  and headers, cannot be zero. This is the combination of %I and %O.
	appendS = func(dst []byte, ctx *fasthttp.RequestCtx) []byte {
		h := len(ctx.Request.Header.RawHeaders())
		b := len(ctx.Request.Body())
		h += len(ctx.Response.Header.Header())
		b += len(ctx.Response.Body())
		return fasthttp.AppendUint(dst, h+b)
	}
)

// Refer to https://httpd.apache.org/docs/2.4/ja/mod/mod_log_config.html
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

type nilAccessLog struct{}

var (
	NilAccessLog nilAccessLog
	_            AccessLog = (*nilAccessLog)(nil)
)

func (nilAccessLog) Collect(ctx *fasthttp.RequestCtx) {}
func (nilAccessLog) Log(ctx *fasthttp.RequestCtx)     {}
func (nilAccessLog) Rotate() error                    { return nil }
func (nilAccessLog) Write([]byte) (int, error)        { return 0, nil }
func (nilAccessLog) Close() error                     { return nil }
