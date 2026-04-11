package accesslog

import (
	"bufio"
	"io"
	"sync"
	"time"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/valyala/fasthttp"
)

var (
	// HeaderKeyOriginalRequestURI is a key to store original RequestURI.
	HeaderKeyOriginalRequestURI = []byte("Original-Request-URI")
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
	if cfg.AccessLog.Output == "" {
		return NilAccessLog, nil
	}
	out, err := logger.SharedRotator(cfg.AccessLog.Output, cfg.AccessLog.Rotation)
	if err != nil {
		return nil, err
	}
	l, err := newAccessLog(out, cfg)
	if err != nil {
		_ = out.Close()
		return nil, err
	}
	return l, nil
}

// newAccessLog dispatches to the appropriate per-format constructor.
func newAccessLog(out logger.Rotator, cfg config.Config) (*accessLog, error) {
	if cfg.AccessLog.Format == FormatJSON {
		return newJSONAccessLog(out, cfg)
	}
	return newNCSAAccessLog(out, cfg)
}

type accessLog struct {
	out               logger.Rotator
	appendLine        appendFunc
	collectRequestURI bool
	bufPool           sync.Pool
	mu                sync.Mutex
	bw                *bufio.Writer
	done              chan struct{}
	closed            bool
}

// newSkeleton allocates an *accessLog with the shared pipeline state
// (bufio.Writer, sync.Pool, done channel). The caller is responsible for
// setting appendLine before invoking startFlushLoop.
func newSkeleton(out logger.Rotator, cfg config.Config) *accessLog {
	bufSize := cfg.AccessLog.BufferSize
	if bufSize <= 0 {
		bufSize = 4096
	}
	return &accessLog{
		out:  out,
		bw:   bufio.NewWriterSize(out, bufSize),
		done: make(chan struct{}),
		bufPool: sync.Pool{
			New: func() any {
				b := make([]byte, 0, 256)
				return &b
			},
		},
	}
}

// startFlushLoop launches the periodic flush goroutine. Call this once
// after appendLine has been set.
func (l *accessLog) startFlushLoop(cfg config.Config) {
	flushInterval := time.Duration(cfg.AccessLog.FlushInterval) * time.Millisecond
	if flushInterval <= 0 {
		flushInterval = time.Second
	}
	go l.flushLoop(flushInterval)
}

var timeNow = func() time.Time { return time.Now() }

// Rotate rotate log stream.
func (l *accessLog) Rotate() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.bw.Flush(); err != nil {
		return err
	}

	return l.out.Rotate()
}

// Write writes to log stream.
func (l *accessLog) Write(p []byte) (int, error) {
	return l.out.Write(p)
}

// Close closes log stream.
func (l *accessLog) Close() error {
	if l.closed {
		return nil
	}

	l.closed = true
	close(l.done)

	l.mu.Lock()
	l.bw.Flush() //nolint:errcheck // best-effort flush on close
	l.mu.Unlock()

	l.appendLine = nil
	l.collectRequestURI = false
	if out := l.out; out != nil {
		l.out = logger.NilRotator
		if err := out.Close(); err != nil {
			return err
		}
	}
	return nil
}

// Collect stores Request-URI to ctx as UserValue if '%r' is specified in format.
func (l *accessLog) Collect(ctx *fasthttp.RequestCtx) {
	if l.collectRequestURI {
		ctx.Request.Header.SetBytesKV(HeaderKeyOriginalRequestURI, ctx.RequestURI())
	}
}

func (l *accessLog) flushLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-l.done:
			return
		case <-ticker.C:
			l.mu.Lock()
			l.bw.Flush() //nolint:errcheck // periodic best-effort flush
			l.mu.Unlock()
		}
	}
}

func (l *accessLog) Log(ctx *fasthttp.RequestCtx) {
	if l.closed {
		return
	}

	bp := l.bufPool.Get().(*[]byte)
	buf := (*bp)[:0]

	buf = l.appendLine(buf, ctx)
	buf = append(buf, '\n')

	l.mu.Lock()
	_, _ = l.bw.Write(buf)
	l.mu.Unlock()

	*bp = buf
	l.bufPool.Put(bp)
}

// appendFunc is the signature shared by every per-token NCSA helper and the
// JSON whole-line writer.
type appendFunc func(dst []byte, ctx *fasthttp.RequestCtx) []byte

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
