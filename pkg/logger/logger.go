package logger

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/valyala/fasthttp"
	"gopkg.in/natefinch/lumberjack.v2"
)

var global Logger

// Global returns a logger that is set as global. If the global logger is not
// set then it returns NilLogger.
func Global() Logger {
	if global == nil {
		return NilLogger
	}
	return global
}

// SetGlobal sets a logger as global logger.
func SetGlobal(l Logger) {
	global = l
}

// Rotater is the interface that defines the methods to rotate files.
type Rotater interface {
	Rotate() error
}

// Logger is the interface that defines the methods to logging.
type Logger interface {
	fasthttp.Logger
	Rotater
	io.Closer
	LogLogger() *log.Logger
}

// WriteRotateCloser is the interface that groups the Rotate and basic Write,
// Close methods.
type WriteRotateCloser interface {
	io.WriteCloser
	Rotater
}

// NopWriteRotateCloser is a Writer with no-op Rotate and Close methods.
type NopWriteRotateCloser struct {
	io.Writer
}

// Rotate does nothing, returns nil.
func (*NopWriteRotateCloser) Rotate() error {
	return nil
}

// Close does nothing, returns nil.
func (*NopWriteRotateCloser) Close() error {
	return nil
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
		return NewLoggerWriteRotateCloser(cfg, &lumberjack.Logger{
			Filename:   cfg.Output,
			MaxSize:    cfg.Rotation.MaxSize,
			MaxBackups: cfg.Rotation.MaxBackups,
			MaxAge:     cfg.Rotation.MaxAge,
			Compress:   cfg.Rotation.Compress,
			LocalTime:  cfg.Rotation.LocalTime,
		})
	}
}

// NewLoggerWriter returns a new logger with out.
func NewLoggerWriter(cfg config.Log, out io.Writer) (Logger, error) {
	return newLogger(&NopWriteRotateCloser{Writer: out}, cfg)
}

// NewLoggerWriteRotateCloser returns a new logger with out.
func NewLoggerWriteRotateCloser(cfg config.Log, out WriteRotateCloser) (Logger, error) {
	return newLogger(out, cfg)
}

type logger struct {
	*log.Logger
	rotateCloser WriteRotateCloser
}

func newLogger(out WriteRotateCloser, cfg config.Log) (*logger, error) {
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
	return &logger{Logger: log.New(out, cfg.Prefix, flgs), rotateCloser: out}, nil
}

// Rotate rotate log stream.
func (l *logger) Rotate() error {
	if l.rotateCloser != nil {
		return l.rotateCloser.Rotate()
	}
	return nil
}

// Close closes log stream.
func (l *logger) Close() error {
	if l.rotateCloser != nil {
		if err := l.rotateCloser.Close(); err != nil {
			return err
		}
		l.rotateCloser = nil
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
func (nilLogger) Close() error                              { return nil }
func (nilLogger) LogLogger() *log.Logger                    { return NilLogLogger }
