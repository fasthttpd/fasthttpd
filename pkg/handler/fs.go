package handler

import (
	"errors"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/mojatter/tree"
	"github.com/mojatter/tree/schema"
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
	config.RegisterHandlerSchema("fs", fsSchemas)
}

// fsSchemas describes the config fields accepted by the fs handler.
// Fields mirror fasthttp.FS as consumed via tree.UnmarshalViaJSON;
// the Map.KeyedRules at "." pins the allow-list so unlisted keys
// surface as "unknown key" errors.
var fsSchemas = schema.QueryRules{
	".": schema.Map{KeyedRules: map[string]schema.Rule{
		"type":                 schema.String{Enum: []string{"fs"}},
		"root":                 schema.String{},
		"compressRoot":         schema.String{},
		"compressedFileSuffix": schema.String{},
		"indexNames":           schema.Array{},
		"cacheDuration":        config.DurationRule{},
		"allowEmptyRoot":       schema.Bool{},
		"compress":             schema.Bool{},
		"compressBrotli":       schema.Bool{},
		"compressZstd":         schema.Bool{},
		"generateIndexPages":   schema.Bool{},
		"acceptByteRange":      schema.Bool{},
		"skipCache":            schema.Bool{},
		"compressedFileSuffixes": schema.Every{Rules: schema.QueryRules{
			".": schema.String{},
		}},
	}},
	".indexNames[]": schema.String{},
}
