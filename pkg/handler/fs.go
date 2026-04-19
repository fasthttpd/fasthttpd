package handler

import (
	"errors"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/mojatter/tree"
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
// keys not listed here fail Validate under strict mode.
var fsSchemas = map[string]config.Schema{
	".type":                   config.StringSchema{Enum: []string{"fs"}},
	".root":                   config.StringSchema{},
	".compressRoot":           config.StringSchema{},
	".compressedFileSuffix":   config.StringSchema{},
	".indexNames":             config.ArraySchema{},
	".indexNames[]":           config.StringSchema{},
	".cacheDuration":          config.DurationSchema{},
	".allowEmptyRoot":         config.BoolSchema{},
	".compress":               config.BoolSchema{},
	".compressBrotli":         config.BoolSchema{},
	".compressZstd":           config.BoolSchema{},
	".generateIndexPages":     config.BoolSchema{},
	".acceptByteRange":        config.BoolSchema{},
	".skipCache":              config.BoolSchema{},
	".compressedFileSuffixes": config.MapOfSchema{Value: config.StringSchema{}},
}
