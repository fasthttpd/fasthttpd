package handler

import (
	"bytes"
	"math/rand"
	"strings"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/mojatter/tree"
	"github.com/valyala/fasthttp"
)

// Content represents a handler that provides a simple content.
//
// The config tree is walked once at construction time and the result is
// stored as plain Go values so that Handle never reaches back into the
// tree.Map hierarchy on the hot path. This keeps per-request work at
// zero allocations, matching the fasthttp baseline.
type Content struct {
	defaultResponse contentResponse
	conditions      []contentCondition
}

type contentHeader struct {
	key, value []byte
}

type contentResponse struct {
	headers    []contentHeader
	body       []byte
	statusCode int
	hasStatus  bool
}

type contentConditionKind uint8

const (
	condKindNone contentConditionKind = iota
	condKindPath
	condKindQuery
	condKindPercentage
)

type contentQueryMatch struct {
	key, value []byte
}

type contentCondition struct {
	kind         contentConditionKind
	path         []byte
	queryMatches []contentQueryMatch
	percentage   int
	response     contentResponse
}

var _ fasthttp.RequestHandler = (*Content)(nil).Handle

// NewContent creates a new Content that provides a simple content.
// The handlerCfg can be specified 'body' string and 'headers' map.
func NewContent(handlerCfg tree.Map) (*Content, error) {
	h := &Content{
		defaultResponse: buildContentResponse(handlerCfg, nil),
	}
	if conds := handlerCfg.Get("conditions").Array(); len(conds) > 0 {
		h.conditions = make([]contentCondition, 0, len(conds))
		for _, c := range conds {
			h.conditions = append(h.conditions, buildContentCondition(c.Map(), handlerCfg))
		}
	}
	return h, nil
}

// buildContentResponse extracts body / headers / status from cfg. When cfg
// does not declare headers, it inherits them from fallback (the root
// handler config), matching the pre-refactor fallback rule.
func buildContentResponse(cfg, fallback tree.Map) contentResponse {
	r := contentResponse{}
	if cfg.Has("body") {
		r.body = []byte(cfg.Get("body").Value().String())
	}
	if cfg.Has("status") {
		r.hasStatus = true
		r.statusCode = cfg.Get("status").Value().Int()
	}
	headers := cfg.Get("headers")
	if headers.IsNil() && fallback != nil {
		headers = fallback.Get("headers")
	}
	r.headers = appendContentHeaders(nil, headers)
	return r
}

func appendContentHeaders(dst []contentHeader, headers tree.Node) []contentHeader {
	switch headers.Type() {
	case tree.TypeMap:
		for k, v := range headers.Map() {
			dst = append(dst, contentHeader{
				key:   []byte(k),
				value: []byte(v.Value().String()),
			})
		}
	case tree.TypeArray:
		for _, v := range headers.Array() {
			switch v.Type() {
			case tree.TypeStringValue:
				kv := strings.SplitN(v.Value().String(), ": ", 2)
				if len(kv) == 2 {
					dst = append(dst, contentHeader{
						key:   []byte(kv[0]),
						value: []byte(kv[1]),
					})
				}
			case tree.TypeMap:
				for kk, vv := range v.Map() {
					dst = append(dst, contentHeader{
						key:   []byte(kk),
						value: []byte(vv.Value().String()),
					})
				}
			}
		}
	}
	return dst
}

func buildContentCondition(cfg, fallback tree.Map) contentCondition {
	cc := contentCondition{response: buildContentResponse(cfg, fallback)}
	switch {
	case cfg.Has("path"):
		cc.kind = condKindPath
		cc.path = []byte(cfg.Get("path").Value().String())
	case cfg.Has("queryStringContains"):
		cc.kind = condKindQuery
		args := &fasthttp.Args{}
		args.Parse(cfg.Get("queryStringContains").Value().String())
		for k, v := range args.All() {
			cc.queryMatches = append(cc.queryMatches, contentQueryMatch{
				key:   append([]byte(nil), k...),
				value: append([]byte(nil), v...),
			})
		}
	default:
		if pct := cfg.Get("percentage").Value().Int(); pct > 0 {
			cc.kind = condKindPercentage
			cc.percentage = pct
		}
	}
	return cc
}

var contentRandomPercentage = func() int {
	return rand.Intn(100)
}

// Handle sets headers and body to the provided ctx.
func (h *Content) Handle(ctx *fasthttp.RequestCtx) {
	for i := range h.conditions {
		cond := &h.conditions[i]
		switch cond.kind {
		case condKindPath:
			if bytes.Equal(ctx.Path(), cond.path) {
				cond.response.writeTo(ctx)
				return
			}
		case condKindQuery:
			args := ctx.Request.URI().QueryArgs()
			matches := true
			for j := range cond.queryMatches {
				qm := &cond.queryMatches[j]
				if !bytes.Equal(args.PeekBytes(qm.key), qm.value) {
					matches = false
					break
				}
			}
			if matches {
				cond.response.writeTo(ctx)
				return
			}
		case condKindPercentage:
			if contentRandomPercentage() <= cond.percentage {
				cond.response.writeTo(ctx)
				return
			}
		}
	}
	h.defaultResponse.writeTo(ctx)
}

func (r *contentResponse) writeTo(ctx *fasthttp.RequestCtx) {
	for i := range r.headers {
		hdr := &r.headers[i]
		ctx.Response.Header.AddBytesKV(hdr.key, hdr.value)
	}
	if r.body != nil {
		ctx.Response.SetBodyRaw(r.body)
	}
	if r.hasStatus {
		ctx.Response.SetStatusCode(r.statusCode)
	}
}

// NewContentHandler creates a new fasthttp.RequestHandler via Content.Handle.
func NewContentHandler(cfg tree.Map, _ logger.Logger) (fasthttp.RequestHandler, error) {
	h, err := NewContent(cfg)
	if err != nil {
		return nil, err
	}
	return h.Handle, nil
}

func init() {
	RegisterNewHandlerFunc("content", NewContentHandler)
	config.RegisterHandlerSchema("content", contentSchemas)
}

// contentSchemas covers the content handler's declarative response.
// `headers` uses AnySchema because the handler accepts both map form
// ({K: V}) and array form (["K: V", ...]); a tighter rule would
// reject one of them. Condition-level fields mirror the defaults.
var contentSchemas = map[string]config.Schema{
	".type":       config.StringSchema{Enum: []string{"content"}},
	".body":       config.StringSchema{},
	".status":     config.IntSchema{Min: config.Int64Ptr(100), Max: config.Int64Ptr(599)},
	".headers":    config.AnySchema{},
	".conditions": config.ArraySchema{},

	".conditions[].path":                config.StringSchema{},
	".conditions[].queryStringContains": config.StringSchema{},
	".conditions[].percentage":          config.IntSchema{Min: config.Int64Ptr(0), Max: config.Int64Ptr(100)},
	".conditions[].body":                config.StringSchema{},
	".conditions[].status":              config.IntSchema{Min: config.Int64Ptr(100), Max: config.Int64Ptr(599)},
	".conditions[].headers":             config.AnySchema{},
}
