package net

import (
	"crypto/tls"
	"errors"
	"os"
	"testing"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/util"
	"golang.org/x/crypto/acme"
)

func TestMultiTLSConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "*.tls")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cfgs := []config.Config{
		{
			SSL: config.SSL{
				AutoCert:         true,
				AutoCertCacheDir: tmpDir,
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
	cert1, err := tls.LoadX509KeyPair("../../examples/ssl/localhost.crt", "../../examples/ssl/localhost.key")
	if err != nil {
		t.Fatal(err)
	}
	cert2, err := tls.LoadX509KeyPair("../../examples/ssl/127.0.0.1.crt", "../../examples/ssl/127.0.0.1.key")
	if err != nil {
		t.Fatal(err)
	}
	fn := func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		if hello.ServerName == "127.0.0.1" {
			return &cert2, nil
		}
		return nil, errors.New("test error")
	}
	m := &multiTlsCert{
		cfg: &tls.Config{
			NextProtos:   []string{"http/1.1", acme.ALPNProto},
			Certificates: []tls.Certificate{cert1},
		},
		fns: []func(*tls.ClientHelloInfo) (*tls.Certificate, error){fn},
	}

	tests := []struct {
		hello  *tls.ClientHelloInfo
		want   tls.Certificate
		errstr string
	}{
		{
			hello: &tls.ClientHelloInfo{
				ServerName:        "localhost",
				SignatureSchemes:  []tls.SignatureScheme{tls.PSSWithSHA256},
				SupportedVersions: []uint16{tls.VersionTLS13},
			},
			want: cert1,
		}, {
			hello: &tls.ClientHelloInfo{ServerName: "127.0.0.1"},
			want:  cert2,
		}, {
			hello: &tls.ClientHelloInfo{ServerName: "example.com"},
			want:  cert1,
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
		if got == nil {
			if len(test.want.Certificate) > 0 {
				t.Errorf("tests[%d] got %v; want %v", i, got, test.want)
			}
			continue
		}
		if !util.Bytes2dEqual(got.Certificate, test.want.Certificate) {
			t.Errorf("tests[%d] got %v; want %v", i, got, test.want)
		}
	}
}
