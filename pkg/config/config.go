package config

import (
	"bytes"
	"io"
	"io/ioutil"
	"time"

	"github.com/jarxorg/tree"
	"gopkg.in/yaml.v2"
)

const (
	DefaultListen     = ":8080"
	DefaultServerName = "fasthttpd"
	MatchPrefix       = "prefix"
	MatchEqual        = "equal"
	MatchRegexp       = "regexp"
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

func (cfg Config) SetDefaults() Config {
	if cfg.Listen == "" {
		cfg.Listen = DefaultListen
	}
	if cfg.Server == nil {
		cfg.Server = tree.Map{}
	}
	if !cfg.Server.Has("name") {
		cfg.Server.Set("name", tree.ToValue(DefaultServerName)) //nolint:errcheck
	}
	cfg.Log = cfg.Log.SetDefaults()
	cfg.AccessLog = cfg.AccessLog.SetDefaults()
	return cfg
}

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
	return cfg, nil
}

// SSL represents a configuration of SSL.
type SSL struct {
	CertFile string `yaml:"certFile"`
	KeyFile  string `yaml:"keyFile"`
}

// Rotation represents a configuration of log rotation.
type Rotation struct {
	MaxSize    int  `yaml:"maxSize"`
	MaxBackups int  `yaml:"maxBackups"`
	MaxAge     int  `yaml:"maxAge"`
	Compress   bool `yaml:"compress"`
	LocalTime  bool `yaml:"localTime"`
}

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
	Enable bool `yaml:"enable"`
	Expire int  `yaml:"expire"`
}

func UnmarshalYAMLPath(path string) ([]Config, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return UnmarshalYAML(data)
}

func UnmarshalYAML(data []byte) ([]Config, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var cfgs []Config
	for {
		cfg := Config{}.SetDefaults()
		err := dec.Decode(&cfg)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		cfg, err = cfg.Normalize()
		if err != nil {
			return nil, err
		}
		cfgs = append(cfgs, cfg)
	}
	return cfgs, nil
}
