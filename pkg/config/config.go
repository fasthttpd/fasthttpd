package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mojatter/tree"
	"github.com/valyala/fasthttp"
	"go.yaml.in/yaml/v3"
)

// Default values.
const (
	DefaultListen          = ":8080"
	DefaultServerName      = "fasthttpd"
	DefaultShutdownTimeout = "30s"
)

// Supported Route.Match values.
const (
	MatchPrefix = "prefix"
	MatchEqual  = "equal"
	MatchRegexp = "regexp"
)

// Config represents a configuration root of fasthttpd.
type Config struct {
	Host            string              `yaml:"host" json:"host"`
	Listen          string              `yaml:"listen" json:"listen"`
	SSL             SSL                 `yaml:"ssl" json:"ssl"`
	Root            string              `yaml:"root" json:"root"`
	Server          Server              `yaml:"server" json:"server"`
	Log             Log                 `yaml:"log" json:"log"`
	AccessLog       AccessLog           `yaml:"accessLog" json:"accessLog"`
	ErrorPages      map[string]string   `yaml:"errorPages" json:"errorPages"`
	Filters         map[string]tree.Map `yaml:"filters" json:"filters"`
	Handlers        map[string]tree.Map `yaml:"handlers" json:"handlers"`
	Routes          []Route             `yaml:"routes" json:"routes"`
	RoutesCache     RoutesCache         `yaml:"routesCache" json:"routesCache"`
	ShutdownTimeout string              `yaml:"shutdownTimeout" json:"shutdownTimeout"`
}

// SetDefaults sets default values.
func (cfg Config) SetDefaults() Config {
	cfg.Listen = DefaultListen
	cfg.Server = Server{Name: DefaultServerName}
	cfg.ShutdownTimeout = DefaultShutdownTimeout
	cfg.SSL = cfg.SSL.SetDefaults()
	cfg.Log = cfg.Log.SetDefaults()
	cfg.AccessLog = cfg.AccessLog.SetDefaults()
	return cfg
}

// Normalize normalizes values.
func (cfg Config) Normalize() (Config, error) {
	var err error
	if cfg.SSL, err = cfg.SSL.Normalize(); err != nil {
		return cfg, err
	}
	if cfg.ShutdownTimeout != "" {
		if _, err := time.ParseDuration(cfg.ShutdownTimeout); err != nil {
			return cfg, fmt.Errorf("failed to parse shutdownTimeout: %w", err)
		}
	}
	for _, route := range cfg.Routes {
		if route.Handler != "" {
			if _, ok := cfg.Handlers[route.Handler]; !ok {
				return cfg, fmt.Errorf("unknown handler %q", route.Handler)
			}
		}
	}
	return cfg, nil
}

// Size wraps a byte count so YAML input may be either a string with
// a binary unit suffix ("4k", "8 KiB", "2M") or a plain integer
// literal (4096). Accepted suffixes are K/M/G (case-insensitive),
// optionally followed by `i` and/or `B`, all interpreted as powers
// of 1024.
type Size int64

var sizeRe = regexp.MustCompile(`^\s*(\d+)\s*([KMG]?)[Ii]?[Bb]?\s*$`)

// parseSize parses strings like "4K", "8kib", "2 MB" into a byte
// count using binary units (K=1024, M=1024^2, G=1024^3). A bare
// integer ("4096") is also accepted.
func parseSize(s string) (int64, error) {
	m := sizeRe.FindStringSubmatch(strings.ToUpper(s))
	if m == nil {
		return 0, fmt.Errorf("invalid size %q", s)
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", s, err)
	}
	switch m[2] {
	case "K":
		n <<= 10
	case "M":
		n <<= 20
	case "G":
		n <<= 30
	}
	return n, nil
}

// UnmarshalYAML accepts "!!str" (parsed via parseSize) or "!!int"
// (interpreted as a byte count).
func (s *Size) UnmarshalYAML(value *yaml.Node) error {
	switch value.Tag {
	case "!!str":
		n, err := parseSize(value.Value)
		if err != nil {
			return err
		}
		*s = Size(n)
	case "!!int":
		var n int64
		if err := value.Decode(&n); err != nil {
			return err
		}
		*s = Size(n)
	default:
		return fmt.Errorf("size must be string or integer, got tag %s", value.Tag)
	}
	return nil
}

// Duration wraps time.Duration so YAML input may be either a string
// parseable by time.ParseDuration ("60s") or an integer nanoseconds
// literal. MarshalJSON emits the underlying int64 so a round-trip
// through fasthttp.Server (which uses time.Duration) is lossless.
type Duration time.Duration

// UnmarshalYAML accepts "!!str" (parsed via time.ParseDuration) or
// "!!int" (interpreted as nanoseconds).
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	switch value.Tag {
	case "!!str":
		p, err := time.ParseDuration(value.Value)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", value.Value, err)
		}
		*d = Duration(p)
	case "!!int":
		var n int64
		if err := value.Decode(&n); err != nil {
			return err
		}
		*d = Duration(n)
	default:
		return fmt.Errorf("duration must be string or integer, got tag %s", value.Tag)
	}
	return nil
}

// Server mirrors the configurable fields of fasthttp.Server. Handler
// callbacks and other function-typed fields are intentionally omitted
// (fasthttpd wires those itself); keepHijackedConns is also omitted
// because fasthttpd does not expose Hijack, and getOnly is omitted
// because method restriction is expressed via Routes.
type Server struct {
	Name               string `yaml:"name" json:"name"`
	Concurrency        int    `yaml:"concurrency" json:"concurrency"`
	ReadBufferSize     Size   `yaml:"readBufferSize" json:"readBufferSize"`
	WriteBufferSize    Size   `yaml:"writeBufferSize" json:"writeBufferSize"`
	MaxConnsPerIP      int    `yaml:"maxConnsPerIP" json:"maxConnsPerIP"`
	MaxRequestsPerConn int    `yaml:"maxRequestsPerConn" json:"maxRequestsPerConn"`
	MaxRequestBodySize Size   `yaml:"maxRequestBodySize" json:"maxRequestBodySize"`

	ReadTimeout                        Duration `yaml:"readTimeout" json:"readTimeout"`
	WriteTimeout                       Duration `yaml:"writeTimeout" json:"writeTimeout"`
	IdleTimeout                        Duration `yaml:"idleTimeout" json:"idleTimeout"`
	MaxIdleWorkerDuration              Duration `yaml:"maxIdleWorkerDuration" json:"maxIdleWorkerDuration"`
	TCPKeepalivePeriod                 Duration `yaml:"tcpKeepalivePeriod" json:"tcpKeepalivePeriod"`
	SleepWhenConcurrencyLimitsExceeded Duration `yaml:"sleepWhenConcurrencyLimitsExceeded" json:"sleepWhenConcurrencyLimitsExceeded"`

	DisableKeepalive              bool `yaml:"disableKeepalive" json:"disableKeepalive"`
	TCPKeepalive                  bool `yaml:"tcpKeepalive" json:"tcpKeepalive"`
	ReduceMemoryUsage             bool `yaml:"reduceMemoryUsage" json:"reduceMemoryUsage"`
	DisablePreParseMultipartForm  bool `yaml:"disablePreParseMultipartForm" json:"disablePreParseMultipartForm"`
	LogAllErrors                  bool `yaml:"logAllErrors" json:"logAllErrors"`
	SecureErrorLogMessage         bool `yaml:"secureErrorLogMessage" json:"secureErrorLogMessage"`
	DisableHeaderNamesNormalizing bool `yaml:"disableHeaderNamesNormalizing" json:"disableHeaderNamesNormalizing"`
	NoDefaultServerHeader         bool `yaml:"noDefaultServerHeader" json:"noDefaultServerHeader"`
	NoDefaultDate                 bool `yaml:"noDefaultDate" json:"noDefaultDate"`
	NoDefaultContentType          bool `yaml:"noDefaultContentType" json:"noDefaultContentType"`
	CloseOnShutdown               bool `yaml:"closeOnShutdown" json:"closeOnShutdown"`
	StreamRequestBody             bool `yaml:"streamRequestBody" json:"streamRequestBody"`
}

// ApplyTo copies Server's configurable fields onto dst. Handler /
// ErrorHandler / Logger / TLSConfig are wired separately and left
// untouched.
func (s Server) ApplyTo(dst *fasthttp.Server) {
	dst.Name = s.Name
	dst.Concurrency = s.Concurrency
	dst.ReadBufferSize = int(s.ReadBufferSize)
	dst.WriteBufferSize = int(s.WriteBufferSize)
	dst.MaxConnsPerIP = s.MaxConnsPerIP
	dst.MaxRequestsPerConn = s.MaxRequestsPerConn
	dst.MaxRequestBodySize = int(s.MaxRequestBodySize)

	dst.ReadTimeout = time.Duration(s.ReadTimeout)
	dst.WriteTimeout = time.Duration(s.WriteTimeout)
	dst.IdleTimeout = time.Duration(s.IdleTimeout)
	dst.MaxIdleWorkerDuration = time.Duration(s.MaxIdleWorkerDuration)
	dst.TCPKeepalivePeriod = time.Duration(s.TCPKeepalivePeriod)
	dst.SleepWhenConcurrencyLimitsExceeded = time.Duration(s.SleepWhenConcurrencyLimitsExceeded)

	dst.DisableKeepalive = s.DisableKeepalive
	dst.TCPKeepalive = s.TCPKeepalive
	dst.ReduceMemoryUsage = s.ReduceMemoryUsage
	dst.DisablePreParseMultipartForm = s.DisablePreParseMultipartForm
	dst.LogAllErrors = s.LogAllErrors
	dst.SecureErrorLogMessage = s.SecureErrorLogMessage
	dst.DisableHeaderNamesNormalizing = s.DisableHeaderNamesNormalizing
	dst.NoDefaultServerHeader = s.NoDefaultServerHeader
	dst.NoDefaultDate = s.NoDefaultDate
	dst.NoDefaultContentType = s.NoDefaultContentType
	dst.CloseOnShutdown = s.CloseOnShutdown
	dst.StreamRequestBody = s.StreamRequestBody
}

// ShutdownTimeoutDuration returns the parsed ShutdownTimeout. A non-positive
// value means no deadline (use context.Background()). Assumes Normalize has
// already validated the string.
func (cfg Config) ShutdownTimeoutDuration() time.Duration {
	if cfg.ShutdownTimeout == "" {
		return 0
	}
	d, _ := time.ParseDuration(cfg.ShutdownTimeout)
	return d
}

// SSL represents a configuration of SSL. If AutoCert is true, CertFile and
// KeyFile are ignored.
type SSL struct {
	CertFile         string `yaml:"certFile" json:"certFile"`
	KeyFile          string `yaml:"keyFile" json:"keyFile"`
	AutoCert         bool   `yaml:"autoCert" json:"autoCert"`
	AutoCertCacheDir string `yaml:"autoCertCacheDir" json:"autoCertCacheDir"`
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
	MaxSize    int  `yaml:"maxSize" json:"maxSize"`
	MaxBackups int  `yaml:"maxBackups" json:"maxBackups"`
	MaxAge     int  `yaml:"maxAge" json:"maxAge"`
	Compress   bool `yaml:"compress" json:"compress"`
	LocalTime  bool `yaml:"localTime" json:"localTime"`
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
	Output   string   `yaml:"output" json:"output"`
	Prefix   string   `yaml:"prefix" json:"prefix"`
	Flags    []string `yaml:"flags" json:"flags"`
	Rotation Rotation `json:"rotation"`
}

// SetDefaults sets default values.
func (l Log) SetDefaults() Log {
	l.Flags = []string{"date", "time"}
	l.Rotation = l.Rotation.SetDefaults()
	return l
}

// AccessLog represents a configuration of access log.
type AccessLog struct {
	Output        string   `yaml:"output" json:"output"`
	Format        string   `yaml:"format" json:"format"`
	QueueSize     int      `yaml:"queueSize" json:"queueSize"` // Deprecated: silently ignored. Use BufferSize instead.
	BufferSize    int      `yaml:"bufferSize" json:"bufferSize"`
	FlushInterval int      `yaml:"flushInterval" json:"flushInterval"` // milliseconds
	Rotation      Rotation `json:"rotation"`
}

// SetDefaults sets default values.
func (l AccessLog) SetDefaults() AccessLog {
	l.BufferSize = 4096
	l.FlushInterval = 1000
	l.Rotation = l.Rotation.SetDefaults()
	return l
}

// Route represents a configuration of route.
type Route struct {
	Path                     string   `yaml:"path" json:"path"`
	Match                    string   `yaml:"match" json:"match"`
	Methods                  []string `yaml:"methods" json:"methods"`
	Filters                  []string `yaml:"filters" json:"filters"`
	Rewrite                  string   `yaml:"rewrite" json:"rewrite"`
	RewriteAppendQueryString bool     `yaml:"rewriteAppendQueryString" json:"rewriteAppendQueryString"`
	Handler                  string   `yaml:"handler" json:"handler"`
	Status                   int      `yaml:"status" json:"status"`
	StatusMessage            string   `yaml:"statusMessage" json:"statusMessage"`
	NextIfNotFound           bool     `yaml:"nextIfNotFound" json:"nextIfNotFound"`
}

// RoutesCache represents a configuration of route cache. MaxEntries
// caps the cache at a fixed number of entries; when zero or negative
// the cache is unbounded (pre-existing behavior). When the cap is
// reached, Set on a new key is dropped so that already-cached hot
// paths survive adversarial unique-key floods.
type RoutesCache struct {
	Enable     bool `yaml:"enable" json:"enable"`
	Expire     int  `yaml:"expire" json:"expire"`
	Interval   int  `yaml:"interval" json:"interval"`
	MaxEntries int  `yaml:"maxEntries" json:"maxEntries"`
}

