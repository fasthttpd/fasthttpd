package net

import (
	"crypto/tls"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/util"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// MultiTLSConfig generates multiple TLS config from fasthttpd configrations.
func MultiTLSConfig(cfgs []config.Config) (*tls.Config, error) {
	var certs []tls.Certificate
	var nextProtos util.StringSet
	var fns []func(*tls.ClientHelloInfo) (*tls.Certificate, error)

	for _, cfg := range cfgs {
		if cfg.SSL.AutoCert.Enable {
			log.Printf("enable autoCert, cacheDir: %q", cfg.SSL.AutoCert.CacheDir)
			if err := os.MkdirAll(cfg.SSL.AutoCert.CacheDir, 0700); err != nil {
				return nil, err
			}
			m := &autocert.Manager{
				Prompt:     autocert.AcceptTOS,
				HostPolicy: autocert.HostWhitelist(cfg.Host),
				Cache:      autocert.DirCache(cfg.SSL.AutoCert.CacheDir),
			}
			fns = append(fns, m.GetCertificate)
			nextProtos = nextProtos.Append("http/1.1", acme.ALPNProto)
			continue
		}
		if cfg.SSL.CertFile != "" && cfg.SSL.KeyFile != "" {
			cert, err := tls.LoadX509KeyPair(cfg.SSL.CertFile, cfg.SSL.KeyFile)
			if err != nil {
				return nil, err
			}
			certs = append(certs, cert)
			nextProtos = nextProtos.Append("http/1.1")
			continue
		}
	}
	if len(certs) == 0 && len(fns) == 0 {
		return nil, nil
	}

	cfg := &tls.Config{NextProtos: nextProtos}
	if len(certs) > 0 {
		cfg.Certificates = certs
		cfg.BuildNameToCertificate()
	}
	if len(fns) == 0 {
		return cfg, nil
	}

	return &tls.Config{
		NextProtos:     nextProtos,
		GetCertificate: (&multiTlsCert{cfg: cfg, fns: fns}).GetCertificate,
	}, nil
}

type multiTlsCert struct {
	cfg *tls.Config
	fns []func(*tls.ClientHelloInfo) (*tls.Certificate, error)
	mux sync.Mutex
}

func (m *multiTlsCert) getByName(name string) (*tls.Certificate, bool) {
	// NOTE: The following code is based on "crypt/tls".Config.getCertificate.
	if cert, ok := m.cfg.NameToCertificate[name]; ok {
		return cert, ok
	}
	if len(name) > 0 {
		labels := strings.Split(name, ".")
		labels[0] = "*"
		wildcardName := strings.Join(labels, ".")
		if cert, ok := m.cfg.NameToCertificate[wildcardName]; ok {
			return cert, ok
		}
	}
	return nil, false
}

func (m *multiTlsCert) found(name string, cert *tls.Certificate) *tls.Certificate {
	m.mux.Lock()
	defer m.mux.Unlock()

	if named, ok := m.getByName(name); ok {
		return named
	}
	if m.cfg.NameToCertificate == nil {
		m.cfg.NameToCertificate = make(map[string]*tls.Certificate)
	}
	log.Printf("store new cert %s", name)
	m.cfg.NameToCertificate[name] = cert
	return cert
}

func (m *multiTlsCert) notFound(name string) {
	m.mux.Lock()
	defer m.mux.Unlock()

	if m.cfg.NameToCertificate == nil {
		m.cfg.NameToCertificate = make(map[string]*tls.Certificate)
	}
	log.Printf("store nil cert %s", name)
	m.cfg.NameToCertificate[name] = nil
}

func (m *multiTlsCert) GetCertificate(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	name := strings.ToLower(clientHello.ServerName)
	if named, ok := m.getByName(name); ok {
		return named, nil
	}

	var errs []error
	for _, fn := range m.fns {
		cert, err := fn(clientHello)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		return m.found(name, cert), nil
	}
	m.notFound(name)

	if len(errs) > 0 {
		// TODO: returns first error only
		return nil, errs[0]
	}
	return nil, nil
}
