package logger

import (
	"fmt"
	"io"
	"log"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/valyala/fasthttp"
)

// Logger is the interface that defines the methods to logging.
type Logger interface {
	fasthttp.Logger
	Rotater
	io.Closer
	LogLogger() *log.Logger
}

// NewLogger creates a new logger.
func NewLogger(cfg config.Log) (Logger, error) {
	if cfg.Output == "" {
		return NilLogger, nil
	}
	out, err := SharedRotater(cfg.Output, cfg.Rotation)
	if err != nil {
		return nil, err
	}
	l, err := newLogger(out, cfg)
	if err != nil {
		out.Close() //nolint:errcheck
		return nil, err
	}
	return l, nil
}

// NewLoggerWriter returns a new logger with out.
func NewLoggerWriter(cfg config.Log, out io.Writer) (Logger, error) {
	return newLogger(&NopRotater{Writer: out}, cfg)
}

type logger struct {
	*log.Logger
	rotater Rotater
}

var _ Logger = (*logger)(nil)

func newLogger(out Rotater, cfg config.Log) (*logger, error) {
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
		default:
			return nil, fmt.Errorf("unknown flag: %s", flg)
		}
	}
	return &logger{
		Logger:  log.New(out, cfg.Prefix, flgs),
		rotater: out,
	}, nil
}

// Rotate rotate log stream.
func (l *logger) Rotate() error {
	if l.rotater != nil {
		return l.rotater.Rotate()
	}
	return nil
}

// Write writes to log stream.
func (l *logger) Write(p []byte) (int, error) {
	if l.rotater != nil {
		return l.rotater.Write(p)
	}
	return 0, nil
}

// Close closes log stream.
func (l *logger) Close() error {
	if l.rotater != nil {
		if err := l.rotater.Close(); err != nil {
			return err
		}
		l.rotater = nil
	}
	return nil
}

func (l *logger) LogLogger() *log.Logger {
	return l.Logger
}

type nilLogger struct{}

var (
	NilLogLogger = log.New(io.Discard, "", 0)
	NilLogger    nilLogger
	_            Logger = (*nilLogger)(nil)
)

func (nilLogger) Printf(format string, args ...interface{}) {}
func (nilLogger) Rotate() error                             { return nil }
func (nilLogger) Write([]byte) (int, error)                 { return 0, nil }
func (nilLogger) Close() error                              { return nil }
func (nilLogger) LogLogger() *log.Logger                    { return NilLogLogger }
