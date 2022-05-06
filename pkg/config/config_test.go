package config

import (
	"io/ioutil"
	"reflect"
	"testing"

	"github.com/jarxorg/tree"
)

func TestUnmarshalYAML(t *testing.T) {
	data, err := ioutil.ReadFile("testdata/full.yaml")
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnmarshalYAML(data)
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
		Log: Log{Output: "stderr"},
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
