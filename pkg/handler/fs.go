package handler

import (
	"errors"

	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
)

// NewFS creates a fasthttp.FS. The specified cfg must have 'root' entry.
func NewFS(cfg tree.Map) (*fasthttp.FS, error) {
	fs := &fasthttp.FS{
		PathNotFound: SendDefaultError,
	}
	if err := tree.UnmarshalViaJSON(cfg, fs); err != nil {
		return nil, err
	}
	if fs.Root == "" {
		return nil, errors.New("failed to create FS: require 'root' entry")
	}
	return fs, nil
}

// NewFSHandler creates a new fasthttp.RequestHandler via fasthttp.FS.
func NewFSHandler(cfg tree.Map, l logger.Logger) (fasthttp.RequestHandler, error) {
	fs, err := NewFS(cfg)
	if err != nil {
		return nil, err
	}
	return fs.NewRequestHandler(), nil
}

func init() {
	RegisterNewHandlerFunc("fs", NewFSHandler)
}
