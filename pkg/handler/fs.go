package handler

import (
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
)

func NewFS(cfg tree.Map) (*fasthttp.FS, error) {
	fs := &fasthttp.FS{}
	if err := tree.UnmarshalViaJSON(cfg, fs); err != nil {
		return nil, err
	}
	return fs, nil
}

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
