package net

import (
	"crypto/tls"
	"errors"
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"github.com/fasthttpd/fasthttpd/pkg/config"
)

func Test_MultiTLSConfig(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "*.tls")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cfgs := []config.Config{
		{
			SSL: config.SSL{
				AutoCert: config.AutoCert{
					Enable:   true,
					CacheDir: tmpDir,
				},
			},
		}, {
			SSL: config.SSL{
				CertFile: "../../examples/ssl/localhost.crt",
				KeyFile:  "../../examples/ssl/localhost.key",
			},
		},
	}

	if _, err := MultiTLSConfig(cfgs); err != nil {
		t.Fatal(err)
	}
}

func Test_multiTlsCert_GetCertificate(t *testing.T) {
	cert1 := &tls.Certificate{}
	cert2 := &tls.Certificate{}
	fn := func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		if hello.ServerName == "cert2.example.com" {
			return cert2, nil
		}
		return nil, errors.New("test error")
	}
	m := &multiTlsCert{
		cfg: &tls.Config{
			NameToCertificate: map[string]*tls.Certificate{"cert1.example.com": cert1},
		},
		fns: []func(*tls.ClientHelloInfo) (*tls.Certificate, error){fn},
	}
	wantNamed := map[string]*tls.Certificate{
		"cert1.example.com": cert1,
		"cert2.example.com": cert2,
		"cert3.example.com": nil,
	}

	tests := []struct {
		hello  *tls.ClientHelloInfo
		want   *tls.Certificate
		errstr string
	}{
		{
			hello: &tls.ClientHelloInfo{ServerName: "cert1.example.com"},
			want:  cert1,
		}, {
			hello: &tls.ClientHelloInfo{ServerName: "cert2.example.com"},
			want:  cert2,
		}, {
			hello: &tls.ClientHelloInfo{ServerName: "cert2.example.com"},
			want:  cert2,
		}, {
			hello:  &tls.ClientHelloInfo{ServerName: "cert3.example.com"},
			errstr: "test error",
		}, {
			hello: &tls.ClientHelloInfo{ServerName: "cert3.example.com"},
		},
	}
	for i, test := range tests {
		got, err := m.GetCertificate(test.hello)
		if test.errstr != "" {
			if err == nil {
				t.Fatalf("tests[%d] no error", i)
			}
			if err.Error() != test.errstr {
				t.Errorf("tests[%d] error %q; want %q", i, err.Error(), test.errstr)
			}
			continue
		}
		if err != nil {
			t.Fatalf("tests[%d] error %v", i, err)
		}
		if got != test.want {
			t.Errorf("tests[%d] got %#v; want %#v", i, got, *test.want)
		}
	}

	if !reflect.DeepEqual(m.cfg.NameToCertificate, wantNamed) {
		t.Errorf("unexpected  NameToCertificate %#v; want %#v", m.cfg.NameToCertificate, wantNamed)
	}
}
