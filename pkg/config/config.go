package config

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/jarxorg/tree"
	"gopkg.in/yaml.v2"
)

// Default values.
const (
	DefaultListen     = ":8080"
	DefaultServerName = "fasthttpd"
)

// Supported Route.Match values.
const (
	MatchPrefix = "prefix"
	MatchEqual  = "equal"
	MatchRegexp = "regexp"
)

// Config represents a configuration root of fasthttpd.
type Config struct {
	Host        string              `yaml:"host"`
	Listen      string              `yaml:"listen"`
	SSL         SSL                 `yaml:"ssl"`
	Root        string              `yaml:"root"`
	Server      tree.Map            `yaml:"server"`
	Log         Log                 `yaml:"log"`
	AccessLog   AccessLog           `yaml:"accessLog"`
	ErrorPages  map[string]string   `yaml:"errorPages"`
	Filters     map[string]tree.Map `yaml:"filters"`
	Handlers    map[string]tree.Map `yaml:"handlers"`
	Routes      []Route             `yaml:"routes"`
	RoutesCache RoutesCache         `yaml:"routesCache"`
}

// SetDefaults sets default values.
func (cfg Config) SetDefaults() Config {
	cfg.Listen = DefaultListen
	cfg.Server = tree.Map{"name": tree.ToValue(DefaultServerName)}
	cfg.SSL = cfg.SSL.SetDefaults()
	cfg.Log = cfg.Log.SetDefaults()
	cfg.AccessLog = cfg.AccessLog.SetDefaults()
	return cfg
}

// Normalize normalizes values.
func (cfg Config) Normalize() (Config, error) {
	serverTimeDurationNames := []string{
		"readTimeout",
		"writeTimeout",
		"idleTimeout",
		"maxKeepaliveDuration",
		"maxIdleWorkerDuration",
		"tcpKeepalivePeriod",
		"sleepWhenConcurrencyLimitsExceeded",
	}
	for _, name := range serverTimeDurationNames {
		if cfg.Server.Has(name) {
			v := cfg.Server.Get(name)
			if v.Type().IsStringValue() {
				d, err := time.ParseDuration(v.Value().String())
				if err != nil {
					return cfg, err
				}
				if err := cfg.Server.Set(name, tree.NumberValue(d)); err != nil {
					return cfg, err
				}
			}
		}
	}
	var err error
	if cfg.SSL, err = cfg.SSL.Normalize(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// SSL represents a configuration of SSL. If AutoCert is true, CertFile and
// KeyFile are ignored.
type SSL struct {
	CertFile         string `yaml:"certFile"`
	KeyFile          string `yaml:"keyFile"`
	AutoCert         bool   `yaml:"autoCert"`
	AutoCertCacheDir string `yaml:"autoCertCacheDir"`
}

// SetDefaults sets default values.
func (ssl SSL) SetDefaults() SSL {
	return ssl
}

// Normalize normalizes values.
func (ssl SSL) Normalize() (SSL, error) {
	if ssl.AutoCert {
		if ssl.AutoCertCacheDir == "" {
			dir, err := os.UserCacheDir()
			if err != nil {
				return ssl, err
			}
			ssl.AutoCertCacheDir = filepath.Join(dir, "fasthttpd", "cert")
		}
	}
	return ssl, nil
}

// Rotation represents a configuration of log rotation.
type Rotation struct {
	MaxSize    int  `yaml:"maxSize"`
	MaxBackups int  `yaml:"maxBackups"`
	MaxAge     int  `yaml:"maxAge"`
	Compress   bool `yaml:"compress"`
	LocalTime  bool `yaml:"localTime"`
}

// SetDefaults sets default values.
func (r Rotation) SetDefaults() Rotation {
	r.MaxSize = 100
	r.MaxBackups = 14
	r.MaxAge = 28
	r.Compress = true
	r.LocalTime = true
	return r
}

// Log represents a configuration of logging.
type Log struct {
	Output   string   `yaml:"output"`
	Prefix   string   `yaml:"prefix"`
	Flags    []string `yaml:"flags"`
	Rotation Rotation
}

// SetDefaults sets default values.
func (l Log) SetDefaults() Log {
	l.Flags = []string{"date", "time"}
	l.Rotation = l.Rotation.SetDefaults()
	return l
}

// AccessLog represents a configuration of access log.
type AccessLog struct {
	Output   string `yaml:"output"`
	Format   string `yaml:"format"`
	Rotation Rotation
}

// SetDefaults sets default values.
func (l AccessLog) SetDefaults() AccessLog {
	l.Rotation = l.Rotation.SetDefaults()
	return l
}

// Route represents a configuration of route.
type Route struct {
	Path                     string   `yaml:"path"`
	Match                    string   `yaml:"match"`
	Methods                  []string `yaml:"methods"`
	Filters                  []string `yaml:"filters"`
	Rewrite                  string   `yaml:"rewrite"`
	RewriteAppendQueryString bool     `yaml:"rewriteAppendQueryString"`
	Handler                  string   `yaml:"handler"`
	Status                   int      `yaml:"status"`
	StatusMessage            string   `yaml:"statusMessage"`
}

// RoutesCache represents a configuration of route cache.
type RoutesCache struct {
	Enable   bool `yaml:"enable"`
	Expire   int  `yaml:"expire"`
	Interval int  `yaml:"interval"`
}

// UnmarshalYAMLPath decodes path as multi Config YAML documents file.
func UnmarshalYAMLPath(path string) ([]Config, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return UnmarshalYAML(data)
}

// UnmarshalYAML decodes data as multi Config YAML documents.
func UnmarshalYAML(data []byte) ([]Config, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var cfgs []Config
	for {
		cfg := Config{}.SetDefaults()
		if err := dec.Decode(&cfg); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		var err error
		cfg, err = cfg.Normalize()
		if err != nil {
			return nil, err
		}
		cfgs = append(cfgs, cfg)
	}
	return cfgs, nil
}
