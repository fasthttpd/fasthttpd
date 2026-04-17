package filter

import (
	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/mojatter/tree"
	"github.com/valyala/fasthttp"
)

// NewHeaderFilter returns a new HeaderFilter.
func NewHeaderFilter(cfg tree.Map) (Filter, error) {
	return &HeaderFilter{
		request:  newHeaderHandler(cfg.Get("request").Map()),
		response: newHeaderHandler(cfg.Get("response").Map()),
	}, nil
}

// HeaderFilter implements the Filter that filters headers of request and response.
type HeaderFilter struct {
	request  *headerFilter
	response *headerFilter
}

// Request filters ctx.Request.Header.
func (f *HeaderFilter) Request(ctx *fasthttp.RequestCtx) bool {
	f.request.filter(&ctx.Request.Header)
	return true
}

// Response filters ctx.Response.Header.
func (f *HeaderFilter) Response(ctx *fasthttp.RequestCtx) bool {
	f.response.filter(&ctx.Response.Header)
	return true
}

// fasthttpHeader defines the methods of fasthttp header.
type fasthttpHeader interface {
	SetBytesKV(k, v []byte)
	AddBytesKV(k, v []byte)
	DelBytes(k []byte)
}

func newHeaderHandler(cfg tree.Map) *headerFilter {
	h := &headerFilter{}
	for _, v := range cfg.Get("del").Array() {
		h.delKeys = append(h.delKeys, []byte(v.Value().String()))
	}
	for k, v := range cfg.Get("set").Map() {
		h.setKeys = append(h.setKeys, []byte(k))
		h.setValues = append(h.setValues, []byte(v.Value().String()))
	}
	for k, v := range cfg.Get("add").Map() {
		h.addKeys = append(h.addKeys, []byte(k))
		h.addValues = append(h.addValues, []byte(v.Value().String()))
	}
	return h
}

// headerFilter stores keys and values to customize the header.
type headerFilter struct {
	setKeys   [][]byte
	setValues [][]byte
	addKeys   [][]byte
	addValues [][]byte
	delKeys   [][]byte
}

func (h *headerFilter) filter(fh fasthttpHeader) {
	for i, k := range h.setKeys {
		fh.SetBytesKV(k, h.setValues[i])
	}
	for i, k := range h.addKeys {
		fh.AddBytesKV(k, h.addValues[i])
	}
	for _, k := range h.delKeys {
		fh.DelBytes(k)
	}
}

func init() {
	RegisterNewFilterFunc("header", NewHeaderFilter)
	config.RegisterFilterSchema("header", headerSchemas)
}

// headerSchemas covers symmetric request/response header rewrites.
// set/add values are arbitrary user-named header keys (MapOfSchema);
// del is a list of header names to strip.
var headerSchemas = map[string]config.Schema{
	".type":         config.StringSchema{Enum: []string{"header"}},
	".request":      config.MapSchema{},
	".request.set":  config.MapOfSchema{Value: config.StringSchema{}},
	".request.add":  config.MapOfSchema{Value: config.StringSchema{}},
	".request.del":  config.ArraySchema{},
	".request.del[]": config.StringSchema{},
	".response":     config.MapSchema{},
	".response.set": config.MapOfSchema{Value: config.StringSchema{}},
	".response.add": config.MapOfSchema{Value: config.StringSchema{}},
	".response.del": config.ArraySchema{},
	".response.del[]": config.StringSchema{},
}
