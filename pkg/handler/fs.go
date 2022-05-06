package handler

import (
	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
)

func NewFSHandler(cfg tree.Map) (fasthttp.RequestHandler, error) {
	fs := &fasthttp.FS{}
	if err := tree.Unmarshal(cfg, fs); err != nil {
		return nil, err
	}
	return fs.NewRequestHandler(), nil
}

func init() {
	RegisterNewHandlerFunc("fs", NewFSHandler)
}
