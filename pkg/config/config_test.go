package config

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jarxorg/tree"
)

func TestUnmarshalYAMLPath(t *testing.T) {
	got, err := UnmarshalYAMLPath("testdata/full.yaml")
	if err != nil {
		t.Fatal(err)
	}
	want := []Config{
		{
			Host:   "localhost",
			Listen: ":8080",
			Root:   "./public",
			Server: tree.Map{
				"name":            tree.ToValue("fasthttpd"),
				"readBufferSize":  tree.ToValue(4096),
				"writeBufferSize": tree.ToValue(4096),
			},
			SSL: SSL{}.SetDefaults(),
			Log: Log{
				Output:   "logs/error.log",
				Flags:    []string{"date", "time"},
				Rotation: Rotation{}.SetDefaults(),
			},
			AccessLog: AccessLog{
				Output:   "logs/access.log",
				Format:   `%h %l %u %t "%r" %>s %b`,
				Rotation: Rotation{}.SetDefaults(),
			},
			ErrorPages: map[string]string{
				"404": "/err/404.html",
				"5xx": "/err/5xx.html",
			},
			Filters: map[string]tree.Map{
				"auth": {
					"type": tree.ToValue("basicAuth"),
					"users": tree.Array{
						tree.Map{
							"name":   tree.ToValue("fast"),
							"secret": tree.ToValue("httpd"),
						},
					},
					"usersFile": tree.ToValue("./users.yaml"),
				},
				"cache": {
					"type": tree.ToValue("header"),
					"response": tree.Map{
						"set": tree.Map{
							"Cache-Control": tree.ToValue("private, max-age=3600"),
						},
					},
				},
			},
			Handlers: map[string]tree.Map{
				"static": {
					"type":               tree.ToValue("fs"),
					"indexNames":         tree.ToArrayValues("index.html"),
					"compress":           tree.ToValue(true),
					"generateIndexPages": tree.ToValue(false),
				},
				"hello": {
					"type": tree.ToValue("content"),
					"headers": tree.Map{
						"Content-Type": tree.ToValue("text/plain; charset=utf-8"),
					},
					"body": tree.ToValue("Hello FastHttpd\n"),
				},
			},
			Routes: []Route{
				{
					Methods:       []string{"PUT", "DELETE", "CONNECT", "OPTIONS", "TRACE", "PATCH"},
					Status:        405,
					StatusMessage: "Method not allowed",
				}, {
					Path:    "/",
					Match:   MatchEqual,
					Handler: "static",
				}, {
					Path:    "/redirect-external",
					Match:   MatchEqual,
					Rewrite: "http://example.com/",
					Status:  302,
				}, {
					Path:                     "/redirect-internal",
					Match:                    MatchEqual,
					Rewrite:                  "/internal?foo=bar",
					RewriteAppendQueryString: true,
					Status:                   302,
				}, {
					Methods: []string{"GET", "HEAD"},
					Filters: []string{"cache"},
					Path:    `.*\.(js|css|jpg|png|gif|ico)$`,
					Match:   MatchRegexp,
					Handler: "static",
				}, {
					Path:    "^/view/(.+)",
					Match:   MatchRegexp,
					Rewrite: "/view?id=$1",
				}, {
					Filters: []string{"auth"},
					Handler: "hello",
				},
			},
			RoutesCache: RoutesCache{
				Enable: true,
				Expire: 60000,
			},
		}, {
			Host:      "localhost",
			Listen:    ":8443",
			Log:       Log{}.SetDefaults(),
			AccessLog: AccessLog{}.SetDefaults(),
			SSL: SSL{
				CertFile: "./ssl/localhost.crt",
				KeyFile:  "./ssl/localhost.key",
				AutoCert: AutoCert{}.SetDefaults(),
			},
			Server: tree.Map{
				"name": tree.ToValue("fasthttpd"),
			},
			Handlers: map[string]tree.Map{
				"backend": {
					"type": tree.ToValue("proxy"),
					"url":  tree.ToValue("http://localhost:8080"),
				},
			},
			Routes: []Route{
				{
					Path:    "/",
					Handler: "backend",
				},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v\nwant %#v", got, want)
	}
}

func TestUnmarshalYAMLPath_Errors(t *testing.T) {
	invalidYaml, err := os.CreateTemp("", "*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(invalidYaml.Name())

	if err := ioutil.WriteFile(invalidYaml.Name(), []byte(":invalid yaml"), os.ModePerm); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		path   string
		errstr string
	}{
		{
			path:   "testdata/not-found.yaml",
			errstr: "open testdata/not-found.yaml: no such file or directory",
		}, {
			path:   invalidYaml.Name(),
			errstr: "yaml: unmarshal errors",
		},
	}
	for i, test := range tests {
		_, err := UnmarshalYAMLPath(test.path)
		if err == nil {
			t.Fatalf("tests[%d] no error", i)
		}
		if !strings.HasPrefix(err.Error(), test.errstr) {
			t.Errorf("tests[%d] got %q; want %q", i, err.Error(), test.errstr)
		}
	}
}

func TestConfig_Normalize(t *testing.T) {
	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		cfg    Config
		want   Config
		errstr string
	}{
		{
			cfg:  Config{},
			want: Config{},
		}, {
			cfg: Config{
				Server: tree.Map{
					"readTimeout": tree.ToValue("60s"),
				},
			},
			want: Config{
				Server: tree.Map{
					"readTimeout": tree.NumberValue(60 * time.Second),
				},
			},
		}, {
			cfg: Config{
				Server: tree.Map{
					"readTimeout": tree.ToValue("invalid duration"),
				},
			},
			errstr: `time: invalid duration "invalid duration"`,
		}, {
			cfg: Config{
				SSL: SSL{
					AutoCert: AutoCert{
						Enable: true,
					},
				},
			},
			want: Config{
				SSL: SSL{
					AutoCert: AutoCert{
						Enable:   true,
						CacheDir: filepath.Join(userCacheDir, "fasthttpd", "cert"),
					},
				},
			},
		},
	}
	for i, test := range tests {
		got, err := test.cfg.Normalize()
		if test.errstr != "" {
			if err == nil {
				t.Fatalf("tests[%d] no error", i)
			}
			if !strings.HasPrefix(err.Error(), test.errstr) {
				t.Errorf("tests[%d] got %q; want %q", i, err.Error(), test.errstr)
			}
			continue
		}
		if err != nil {
			t.Fatalf("tests[%d] error %v", i, err)
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("tests[%d] got %#v; want %#v", i, got, test.want)
		}
	}
}
