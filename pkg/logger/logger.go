package logger

import (
	"io"
	"log"
	"os"

	"github.com/valyala/fasthttp"
)

func NewLogger(w io.Writer) fasthttp.Logger {
	return fasthttp.Logger(log.New(w, "", log.LstdFlags))
}

func NewFileLogger(name string) (fasthttp.Logger, error) {
	switch name {
	case "stdout":
		return NewLogger(os.Stdout), nil
	case "stderr", "":
		return NewLogger(os.Stderr), nil
	default:
		// TODO: rotation
		f, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			return nil, err
		}
		return NewLogger(f), nil
	}
}

type nilLogger struct{}

var (
	NilLogger nilLogger
	_         fasthttp.Logger = (*nilLogger)(nil)
)

func (nilLogger) Printf(format string, args ...interface{}) {}
