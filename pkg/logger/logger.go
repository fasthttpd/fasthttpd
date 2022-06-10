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
	Rotator
	io.Closer
	LogLogger() *log.Logger
}

// NewLogger creates a new logger.
func NewLogger(cfg config.Log) (Logger, error) {
	if cfg.Output == "" {
		return NilLogger, nil
	}
	out, err := SharedRotator(cfg.Output, cfg.Rotation)
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
	return newLogger(&NopRotator{Writer: out}, cfg)
}

type logger struct {
	*log.Logger
	rotator Rotator
}

var _ Logger = (*logger)(nil)

func newLogger(out Rotator, cfg config.Log) (*logger, error) {
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
		rotator: out,
	}, nil
}

// Rotate rotate log stream.
func (l *logger) Rotate() error {
	if l.rotator != nil {
		return l.rotator.Rotate()
	}
	return nil
}

// Write writes to log stream.
func (l *logger) Write(p []byte) (int, error) {
	if l.rotator != nil {
		return l.rotator.Write(p)
	}
	return 0, nil
}

// Close closes log stream.
func (l *logger) Close() error {
	if l.rotator != nil {
		if err := l.rotator.Close(); err != nil {
			return err
		}
		l.rotator = nil
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

// LoggerDelegator can delegate the Logger functions.
type LoggerDelegator struct {
	PrintfFunc    func(format string, args ...interface{})
	RotateFunc    func() error
	WriteFunc     func([]byte) (int, error)
	CloseFunc     func() error
	LogLoggerFunc func() *log.Logger
}

func (l *LoggerDelegator) Printf(format string, args ...interface{}) {
	if l.PrintfFunc != nil {
		l.PrintfFunc(format, args...)
	}
}

func (l *LoggerDelegator) Rotate() error {
	if l.RotateFunc != nil {
		return l.RotateFunc()
	}
	return nil
}
func (l *LoggerDelegator) Write(p []byte) (int, error) {
	if l.WriteFunc != nil {
		return l.WriteFunc(p)
	}
	return 0, nil
}
func (l *LoggerDelegator) Close() error {
	if l.CloseFunc != nil {
		return l.CloseFunc()
	}
	return nil
}
func (l *LoggerDelegator) LogLogger() *log.Logger {
	if l.LogLoggerFunc != nil {
		return l.LogLoggerFunc()
	}
	return NilLogLogger
}
