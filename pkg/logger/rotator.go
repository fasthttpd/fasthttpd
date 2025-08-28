package logger

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Rotator is the interface that groups the Rotate and basic Write, Close methods.
type Rotator interface {
	io.WriteCloser
	Rotate() error
}

// NewRotator creates a new rotator.
func NewRotator(output string, cfg config.Rotation) (Rotator, error) {
	switch output {
	case "":
		return &NopRotator{Writer: io.Discard}, nil
	case "stdout":
		return &NopRotator{Writer: os.Stdout}, nil
	case "stderr":
		return &NopRotator{Writer: os.Stderr}, nil
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
	sharedRotators = map[string]*sharedRotator{}
)

type sharedRotator struct {
	Rotator
	shared int
}

var _ Rotator = (*sharedRotator)(nil)

func (r *sharedRotator) share() *sharedRotator {
	r.shared++
	return r
}

func (r *sharedRotator) Close() error {
	r.shared--
	if r.shared > 0 {
		return nil
	}
	removeSharedRotator(r)
	return r.Rotator.Close()
}

// SharedRotator returns a Rotator that is mapped by output.
// If the Rotator mapped by output does not exist, a new Rotator is created.
// If there are duplicate ouput, the backward cfg is ignored.
func SharedRotator(output string, cfg config.Rotation) (Rotator, error) {
	sharedMutex.Lock()
	defer sharedMutex.Unlock()

	if shared, ok := sharedRotators[output]; ok {
		return shared.share(), nil
	}
	r, err := NewRotator(output, cfg)
	if err != nil {
		return nil, err
	}
	shared := &sharedRotator{
		Rotator: r,
	}
	sharedRotators[output] = shared
	return shared.share(), nil
}

// removeSharedRotator removes a shared rotator.
func removeSharedRotator(o Rotator) {
	sharedMutex.Lock()
	defer sharedMutex.Unlock()

	for k, v := range sharedRotators {
		if o == v {
			delete(sharedRotators, k)
			return
		}
	}
}

// RotateShared calls Rotate each shared rotators.
func RotateShared() error {
	sharedMutex.Lock()
	defer sharedMutex.Unlock()

	var errs []error
	for _, o := range sharedRotators {
		if err := o.Rotate(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to rotate: %v", errors.Join(errs...))
	}
	return nil
}

// NopRotator is a Writer with no-op Rotate and Close methods.
type NopRotator struct {
	io.Writer
}

var (
	_          Rotator = (*NopRotator)(nil)
	NilRotator         = &NopRotator{Writer: io.Discard}
)

// Rotate does nothing, returns nil.
func (*NopRotator) Rotate() error {
	return nil
}

// Close does nothing, returns nil.
func (*NopRotator) Close() error {
	return nil
}
