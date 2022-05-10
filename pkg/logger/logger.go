package logger

import (
	"io"
	"log"
	"os"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/jarxorg/io2"
	"github.com/valyala/fasthttp"
)

// Logger is an interface to logging.
type Logger interface {
	fasthttp.Logger
	Close() error
}

// NewLogger returns a new logger.
func NewLogger(cfg config.Log) (Logger, error) {
	switch cfg.Output {
	case "":
		return NilLogger, nil
	case "stdout":
		return NewLoggerWriter(cfg, os.Stdout)
	case "stderr":
		return NewLoggerWriter(cfg, os.Stderr)
	default:
		// TODO: rotation and close.
		f, err := os.OpenFile(cfg.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, err
		}
		return NewLoggerWriteCloser(cfg, f)
	}
}

// NewLoggerWriter returns a new logger with out.
func NewLoggerWriter(cfg config.Log, out io.Writer) (Logger, error) {
	return newLogger(io2.NopWriteCloser(out), cfg)
}

// NewLoggerWriteCloser returns a new logger with out.
func NewLoggerWriteCloser(cfg config.Log, out io.WriteCloser) (Logger, error) {
	return newLogger(out, cfg)
}

type logger struct {
	*log.Logger
	closer io.Closer
}

func newLogger(out io.WriteCloser, cfg config.Log) (*logger, error) {
	flgs := 0
	for _, flg := range cfg.Flags {
		switch flg {
		case "date":
			flgs |= log.Ldate
		case "time":
			flgs |= log.Ltime
		case "microsecond":
			flgs |= log.Lmicroseconds
		case "utc":
			flgs |= log.LUTC
		case "msgprefix":
			flgs |= log.Lmsgprefix
		}
	}
	return &logger{Logger: log.New(out, cfg.Prefix, flgs), closer: out}, nil
}

// Close closes log stream.
func (l *logger) Close() error {
	if l.closer != nil {
		if err := l.closer.Close(); err != nil {
			return err
		}
		l.closer = nil
	}
	return nil
}

type nilLogger struct{}

var (
	NilLogger nilLogger
	_         Logger = (*nilLogger)(nil)
)

func (nilLogger) Printf(format string, args ...interface{}) {}
func (nilLogger) Close() error                              { return nil }
