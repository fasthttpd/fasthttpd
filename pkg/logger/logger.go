package logger

import (
	"io"
	"log"
	"os"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/valyala/fasthttp"
	"gopkg.in/natefinch/lumberjack.v2"
)

type Rotater interface {
	Rotate() error
}

// Logger is an interface to logging.
type Logger interface {
	fasthttp.Logger
	Rotater
	io.Closer
}

type WriteCloseRotater interface {
	io.WriteCloser
	Rotater
}

type NopWriteCloseRotater struct {
	io.Writer
}

func (*NopWriteCloseRotater) Rotate() error {
	return nil
}

func (*NopWriteCloseRotater) Close() error {
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
		return NewLoggerWriteCloseRotater(cfg, &lumberjack.Logger{
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
	return newLogger(&NopWriteCloseRotater{Writer: out}, cfg)
}

// NewLoggerWriteCloseRotater returns a new logger with out.
func NewLoggerWriteCloseRotater(cfg config.Log, out WriteCloseRotater) (Logger, error) {
	return newLogger(out, cfg)
}

type logger struct {
	*log.Logger
	closeRotater WriteCloseRotater
}

func newLogger(out WriteCloseRotater, cfg config.Log) (*logger, error) {
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
	return &logger{Logger: log.New(out, cfg.Prefix, flgs), closeRotater: out}, nil
}

// Rotate rotate log stream.
func (l *logger) Rotate() error {
	if l.closeRotater != nil {
		return l.closeRotater.Rotate()
	}
	return nil
}

// Close closes log stream.
func (l *logger) Close() error {
	if l.closeRotater != nil {
		if err := l.closeRotater.Close(); err != nil {
			return err
		}
		l.closeRotater = nil
	}
	return nil
}

type nilLogger struct{}

var (
	NilLogger nilLogger
	_         Logger = (*nilLogger)(nil)
)

func (nilLogger) Printf(format string, args ...interface{}) {}
func (nilLogger) Rotate() error                             { return nil }
func (nilLogger) Close() error                              { return nil }
