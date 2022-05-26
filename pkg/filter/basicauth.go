package filter

import (
	"bytes"
	"encoding/base64"
	"io/ioutil"
	"net/http"

	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
	"gopkg.in/yaml.v2"
)

const (
	DefaultRealm = "Restricted"
)

// BasicAuthUser represents a basic auth user.
type BasicAuthUser struct {
	Name   string `yaml:"name"`
	Secret string `yaml:"secret"`
	auth   []byte
}

// BasicAuthFilter implements Filter.
type BasicAuthFilter struct {
	Realm     string           `yaml:"realm"`
	Users     []*BasicAuthUser `yaml:"users"`
	UsersFile string           `yaml:"usersFile"`
}

// NewBasicAuthFilter returns a new BasicAuthFilter.
func NewBasicAuthFilter(cfg tree.Map) (Filter, error) {
	f := &BasicAuthFilter{
		Realm: DefaultRealm,
	}
	if err := tree.UnmarshalViaYAML(cfg, f); err != nil {
		return nil, err
	}
	if err := f.init(); err != nil {
		return nil, err
	}
	return f, nil
}

func (f *BasicAuthFilter) init() error {
	if f.UsersFile != "" {
		bin, err := ioutil.ReadFile(f.UsersFile)
		if err != nil {
			return err
		}
		var users []*BasicAuthUser
		if err := yaml.Unmarshal(bin, &users); err != nil {
			return err
		}
		f.Users = append(f.Users, users...)
	}
	for i, u := range f.Users {
		plain := []byte(u.Name + ":" + u.Secret)
		u.auth = make([]byte, base64.StdEncoding.EncodedLen(len(plain)))
		base64.StdEncoding.Encode(u.auth, plain)
		u.Secret = ""
		f.Users[i] = u
	}
	return nil
}

func (f *BasicAuthFilter) unauthorized(ctx *fasthttp.RequestCtx) {
	ctx.Response.SetStatusCode(http.StatusUnauthorized)
	ctx.Response.Header.Set("WWW-Authenticate", "Basic realm="+f.Realm)
}

var basicPrefix = []byte("Basic ")

// Request examines the Authorization header of the given ctx and matches it
// against the user it holds. If the user does not match, it sets 401
// Unauthorized and returns false.
func (f *BasicAuthFilter) Request(ctx *fasthttp.RequestCtx) bool {
	header := ctx.Request.Header.Peek(fasthttp.HeaderAuthorization)
	if len(header) == 0 {
		f.unauthorized(ctx)
		return false
	}
	if !bytes.HasPrefix(header, basicPrefix) {
		ctx.Error("Unknown authorization", http.StatusBadRequest)
		return false
	}
	auth := header[len(basicPrefix):]
	for _, u := range f.Users {
		if bytes.Equal(auth, u.auth) {
			ctx.URI().SetUsername(u.Name)
			return true
		}
	}
	f.unauthorized(ctx)
	return false
}

// Response dose nothing and returns true.
func (f *BasicAuthFilter) Response(ctx *fasthttp.RequestCtx) bool {
	return true
}

func init() {
	RegisterNewFilterFunc("basicAuth", NewBasicAuthFilter)
}
