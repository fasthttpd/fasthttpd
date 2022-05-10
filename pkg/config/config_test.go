package config

import (
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jarxorg/tree"
)

func Test_UnmarshalYAMLPath(t *testing.T) {
	got, err := UnmarshalYAMLPath("testdata/full.yaml")
	if err != nil {
		t.Fatal(err)
	}
	want := Config{
		Host:   "localhost",
		Listen: ":8800",
		Root:   "./public",
		Server: tree.Map{
			"name":            tree.ToValue("fasthttpd"),
			"readBufferSize":  tree.ToValue(4096),
			"writeBufferSize": tree.ToValue(4096),
		},
		Log: Log{
			Output: "stderr",
			Flags:  []string{"date", "time"},
		},
		AccessLog: AccessLog{
			Output: "stdout",
			Format: `%h %l %u %t "%r" %>s %b`,
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
		},
		Handlers: map[string]tree.Map{
			"static": {
				"type":               tree.ToValue("fs"),
				"indexNames":         tree.ToArrayValues("index.html"),
				"compress":           tree.ToValue(true),
				"generateIndexPages": tree.ToValue(false),
			},
			"backend": {
				"type": tree.ToValue("proxy"),
				"url":  tree.ToValue("http://localhost:9000"),
			},
		},
		Routes: []Route{
			{
				Methods: []string{"PUT", "DELETE", "CONNECT", "OPTIONS", "TRACE", "PATCH"},
				Status: Status{
					Code:    405,
					Message: "Method not allowed",
				},
			}, {
				Path:    "/",
				Match:   MatchEqual,
				Handler: "static",
			}, {
				Path:  "/redirect-external",
				Match: MatchEqual,
				Rewrite: Rewrite{
					URI: "http://example.com/",
				},
				Status: Status{
					Code: 302,
				},
			}, {
				Path:  "/redirect-internal",
				Match: MatchEqual,
				Rewrite: Rewrite{
					URI:               "/internal?foo=bar",
					AppendQueryString: true,
				},
				Status: Status{
					Code: 302,
				},
			}, {
				Methods: []string{"GET", "HEAD"},
				Path:    `.*\.(js|css|jpg|png|gif|ico)$`,
				Match:   MatchRegexp,
				Handler: "static",
			}, {
				Path:  "^/view/(.+)",
				Match: MatchRegexp,
				Rewrite: Rewrite{
					URI: "/view?id=$1",
				},
			}, {
				Filters: []string{"auth"},
				Handler: "backend",
			},
		},
		RoutesCache: RoutesCache{
			Enable: true,
			Expire: 60000,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v\nwant %#v", got, want)
	}
}

func Test_UnmarshalYAMLPath_Errors(t *testing.T) {
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

func Test_Config_Normalize(t *testing.T) {
	tests := []struct {
		cfg    Config
		want   Config
		errstr string
	}{
		{
			cfg: Config{},
			want: Config{
				Listen: ":8800",
				Server: tree.Map{
					"name": tree.ToValue("fasthttpd"),
				},
			},
		}, {
			cfg: Config{
				Server: tree.Map{
					"readTimeout": tree.ToValue("60s"),
				},
			},
			want: Config{
				Listen: ":8800",
				Server: tree.Map{
					"name":        tree.ToValue("fasthttpd"),
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
