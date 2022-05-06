package config

import (
	"io/ioutil"
	"time"

	"github.com/jarxorg/tree"
	"gopkg.in/yaml.v2"
)

const (
	DefaultListen     = ":8800"
	DefaultServerName = "fasthttpd"
	MatchPrefix       = "prefix"
	MatchEqual        = "equal"
	MatchRegexp       = "regexp"
)

// Config represents a configuration root of fasthttpd.
type Config struct {
	Host        string              `yaml:"host"`
	Listen      string              `yaml:"listen"`
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

func (cfg Config) Normalize() (Config, error) {
	if cfg.Listen == "" {
		cfg.Listen = DefaultListen
	}
	if !cfg.Server.Has("name") {
		cfg.Server.Set("name", tree.ToValue(DefaultServerName))
	}
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
				cfg.Server.Set(name, tree.NumberValue(d))
			}
		}
	}
	return cfg, nil
}

type Log struct {
	Output string `yaml:"output"`
}

type AccessLog struct {
	Output string `yaml:"output"`
	Format string `yaml:"format"`
}

type Route struct {
	Path    string   `yaml:"path"`
	Match   string   `yaml:"match"`
	Methods []string `yaml:"methods"`
	Filters []string `yaml:"filters"`
	Rewrite Rewrite  `yaml:"rewrite"`
	Handler string   `yaml:"handler"`
	Status  Status   `yaml:"status"`
}

type Rewrite struct {
	URI               string `yaml:"uri"`
	AppendQueryString bool   `yaml:"appendQueryString"`
}

type Status struct {
	Code    int    `yaml:"code"`
	Message string `yaml:"message"`
}

type RoutesCache struct {
	Enable bool `yaml:"enable"`
	Expire int  `yaml:"expire"`
}

func UnmarshalYAMLPath(path string) (Config, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	return UnmarshalYAML(data)
}

func UnmarshalYAML(data []byte) (Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg.Normalize()
}
