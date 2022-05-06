package filter

import (
	"reflect"
	"testing"

	"github.com/jarxorg/tree"
)

func Test_NewBasicAuth(t *testing.T) {
	tests := []struct {
		cfg    tree.Map
		want   *BasicAuth
		errstr string
	}{
		{
			cfg: tree.Map{},
			want: &BasicAuth{
				Realm: DefaultRealm,
			},
		}, {
			cfg: tree.Map{
				"realm": tree.ToValue("staff only"),
			},
			want: &BasicAuth{
				Realm: "staff only",
			},
		}, {
			cfg: tree.Map{
				"users": tree.Array{
					tree.Map{
						"name":   tree.ToValue("fast"),
						"secret": tree.ToValue("httpd"),
					},
				},
			},
			want: &BasicAuth{
				Realm: DefaultRealm,
				Users: []*BasicAuthUser{
					{
						Name: "fast",
						auth: []byte{0x5a, 0x6d, 0x46, 0x7a, 0x64, 0x44, 0x70, 0x6f, 0x64, 0x48, 0x52, 0x77, 0x5a, 0x41, 0x3d, 0x3d},
					},
				},
			},
		}, {
			cfg: tree.Map{
				"users": tree.Array{
					tree.Map{
						"name":   tree.ToValue("fast"),
						"secret": tree.ToValue("httpd"),
					},
				},
				"usersFile": tree.ToValue("../config/testdata/users.yaml"),
			},
			want: &BasicAuth{
				Realm: DefaultRealm,
				Users: []*BasicAuthUser{
					{
						Name: "fast",
						auth: []byte{0x5a, 0x6d, 0x46, 0x7a, 0x64, 0x44, 0x70, 0x6f, 0x64, 0x48, 0x52, 0x77, 0x5a, 0x41, 0x3d, 0x3d},
					}, {
						Name: "user01",
						auth: []byte{0x64, 0x58, 0x4e, 0x6c, 0x63, 0x6a, 0x41, 0x78, 0x4f, 0x6e, 0x4e, 0x6c, 0x59, 0x33, 0x4a, 0x6c, 0x64, 0x44, 0x41, 0x78},
					}, {
						Name: "user02",
						auth: []byte{0x64, 0x58, 0x4e, 0x6c, 0x63, 0x6a, 0x41, 0x79, 0x4f, 0x6e, 0x4e, 0x6c, 0x59, 0x33, 0x4a, 0x6c, 0x64, 0x44, 0x41, 0x79},
					},
				},
				UsersFile: "../config/testdata/users.yaml",
			},
		}, {
			cfg: tree.Map{
				"usersFile": tree.ToValue("not-found.yaml"),
			},
			errstr: "open not-found.yaml: no such file or directory",
		},
	}
	for i, test := range tests {
		got, err := NewBasicAuth(test.cfg)
		if test.errstr != "" {
			if err == nil {
				t.Fatalf("tests[%d] is no error; want %q", i, test.errstr)
			}
			if err.Error() != test.errstr {
				t.Errorf("tests[%d] error %q; want %q", i, err.Error(), test.errstr)
			}
			continue
		}
		if err != nil {
			t.Fatalf("tests[%d] error %v", i, err)
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("tests[%d] got %#v; want %#v", i, *got, *test.want)
		}
	}
}