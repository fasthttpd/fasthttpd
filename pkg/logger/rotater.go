package logger

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Rotater is the interface that groups the Rotate and basic Write, Close methods.
type Rotater interface {
	io.WriteCloser
	Rotate() error
}

// NewRotater creates a new rotater.
func NewRotater(output string, cfg config.Rotation) (Rotater, error) {
	switch output {
	case "":
		return &NopRotater{Writer: io.Discard}, nil
	case "stdout":
		return &NopRotater{Writer: os.Stdout}, nil
	case "stderr":
		return &NopRotater{Writer: os.Stderr}, nil
	default:
		return &lumberjack.Logger{
			Filename:   output,
			MaxSize:    cfg.MaxSize,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge,
			Compress:   cfg.Compress,
			LocalTime:  cfg.LocalTime,
		}, nil
	}
}

var (
	sharedMutex    sync.Mutex
	sharedRotaters = map[string]*sharedRotater{}
)

type sharedRotater struct {
	Rotater
	shared int
}

var _ Rotater = (*sharedRotater)(nil)

func (r *sharedRotater) share() *sharedRotater {
	r.shared++
	return r
}

func (r *sharedRotater) Close() error {
	r.shared--
	if r.shared > 0 {
		return nil
	}
	removeSharedRotater(r)
	return r.Rotater.Close()
}

// SharedRotater returns a Rotater that is mapped by output.
// If the Rotater mapped by output does not exist, a new Rotater is created.
// If there are duplicate ouput, the backward cfg is ignored.
func SharedRotater(output string, cfg config.Rotation) (Rotater, error) {
	sharedMutex.Lock()
	defer sharedMutex.Unlock()

	if shared, ok := sharedRotaters[output]; ok {
		return shared.share(), nil
	}
	r, err := NewRotater(output, cfg)
	if err != nil {
		return nil, err
	}
	shared := &sharedRotater{
		Rotater: r,
	}
	sharedRotaters[output] = shared
	return shared.share(), nil
}

// removeSharedRotater removes a shared rotater.
func removeSharedRotater(o Rotater) {
	sharedMutex.Lock()
	defer sharedMutex.Unlock()

	for k, v := range sharedRotaters {
		if o == v {
			delete(sharedRotaters, k)
			return
		}
	}
}

// RotateShared calls Rotate each shared rotaters.
func RotateShared() error {
	sharedMutex.Lock()
	defer sharedMutex.Unlock()

	var errs []string
	for _, o := range sharedRotaters {
		if err := o.Rotate(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to rotate: %s", strings.Join(errs, "; "))
	}
	return nil
}

// NopRotater is a Writer with no-op Rotate and Close methods.
type NopRotater struct {
	io.Writer
}

var _ Rotater = (*NopRotater)(nil)

// Rotate does nothing, returns nil.
func (*NopRotater) Rotate() error {
	return nil
}

// Close does nothing, returns nil.
func (*NopRotater) Close() error {
	return nil
}
