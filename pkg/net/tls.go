package net

import (
	"crypto/tls"
	"errors"
	"log"
	"os"
	"strings"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/util"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

var (
	errNoCertificates       = errors.New("tls: no certificates configured")
	errNotSupportedWildcard = errors.New("tls: wildcard is not supported")
)

// MultiTLSConfig generates multiple TLS config from fasthttpd configrations.
func MultiTLSConfig(cfgs []config.Config) (*tls.Config, error) {
	var certs []tls.Certificate
	var nextProtos util.StringSet
	var fns []func(*tls.ClientHelloInfo) (*tls.Certificate, error)

	autoCertCacheDirToHosts := map[string][]string{}
	for _, cfg := range cfgs {
		if cfg.SSL.AutoCert {
			if strings.Contains(cfg.Host, "*") {
				return nil, errNotSupportedWildcard
			}
			hosts := autoCertCacheDirToHosts[cfg.SSL.AutoCertCacheDir]
			hosts = append(hosts, cfg.Host)
			autoCertCacheDirToHosts[cfg.SSL.AutoCertCacheDir] = hosts
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
	for cacheDir, hosts := range autoCertCacheDirToHosts {
		log.Printf("Enable autoCert, cacheDir: %q", cacheDir)
		if err := os.MkdirAll(cacheDir, 0700); err != nil {
			return nil, err
		}
		m := &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(hosts...),
			Cache:      autocert.DirCache(cacheDir),
		}
		fns = append(fns, m.GetCertificate)
		nextProtos = nextProtos.Append("http/1.1", acme.ALPNProto)
	}
	if len(certs) == 0 && len(fns) == 0 {
		return nil, nil
	}

	cfg := &tls.Config{NextProtos: nextProtos}
	if len(certs) > 0 {
		cfg.Certificates = certs
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
}

// GetCertificate implements tls.Config.GetCertificate.
func (m *multiTlsCert) GetCertificate(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	// NOTE: The following code is based on "crypt/tls".Config.getCertificate.
	for _, cert := range m.cfg.Certificates {
		if err := clientHello.SupportsCertificate(&cert); err == nil {
			return &cert, nil
		}
	}
	for _, fn := range m.fns {
		if cert, err := fn(clientHello); err == nil {
			return cert, nil
		}
	}
	if len(m.cfg.Certificates) == 0 {
		return nil, errNoCertificates
	}
	// If nothing matches, return the first certificate.
	return &m.cfg.Certificates[0], nil
}
