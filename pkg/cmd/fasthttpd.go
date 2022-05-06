package cmd

import (
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/handler"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
)

const (
	cmd          = "fasthttpd"
	desc         = "FastHttpd is a HTTP server using valyala/fasthttp."
	usage        = cmd + " [flags] [query] ([file...])"
	examplesText = `Examples:
  % fasthttpd -f config.yaml
  % fasthttpd -e root=./public -e listen=:8800
`
)

func newMinimalConfig() (config.Config, error) {
	return config.Config{
		Host: "localhost",
		Root: "./public",
		Handlers: map[string]tree.Map{
			"static": {
				"type":       tree.ToValue("fs"),
				"indexNames": tree.ToArrayValues("index.html"),
			},
		},
		Routes: []config.Route{{Handler: "static"}},
	}.Normalize()
}

type FastHttpd struct {
	flagSet    *flag.FlagSet
	isVersion  bool
	isHelp     bool
	isDaemon   bool
	configFile string
	editExprs  stringList
	server     *fasthttp.Server
}

func NewFastHttpd() *FastHttpd {
	return &FastHttpd{}
}

func (d *FastHttpd) initFlagSet(args []string) error {
	s := flag.NewFlagSet(args[0], flag.ExitOnError)
	d.flagSet = s

	s.BoolVar(&d.isVersion, "v", false, "print version")
	s.BoolVar(&d.isHelp, "h", false, "help for "+cmd)
	s.StringVar(&d.configFile, "f", "", "configuration file")
	s.Var(&d.editExprs, "e", "edit expression (eg. -e KEY=VALUE)")
	s.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s\n\nUsage:\n  %s\n\n", desc, usage)
		fmt.Fprintln(os.Stderr, "Flags:")
		s.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n%s", examplesText)
	}
	return s.Parse(args[1:])
}

func (d *FastHttpd) initConfig() (config.Config, error) {
	var err error
	var cfg config.Config
	if d.configFile != "" {
		cfg, err = config.UnmarshalYAMLPath(d.configFile)
		if err != nil {
			return cfg, err
		}
		if err := os.Chdir(filepath.Dir(d.configFile)); err != nil {
			return cfg, err
		}
	} else {
		cfg, err = newMinimalConfig()
		if err != nil {
			return cfg, err
		}
	}
	if len(d.editExprs) > 0 {
		n, err := tree.Marshal(cfg)
		if err != nil {
			return cfg, err
		}
		for _, expr := range d.editExprs {
			lr := strings.SplitN(expr, "=", 2)
			if len(lr) == 2 {
				if !strings.HasPrefix(lr[0], ".") {
					lr[0] = "." + lr[0]
				}
				if _, err := strconv.Atoi(lr[1]); err != nil && !strings.HasPrefix(lr[1], `"`) {
					lr[1] = strconv.Quote(lr[1])
				}
				expr = lr[0] + "=" + lr[1]
			}
			if err := tree.Edit(&n, expr); err != nil {
				return cfg, err
			}
		}
		if err := tree.Unmarshal(n, &cfg); err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}

func (d *FastHttpd) newServer(cfg config.Config) (*fasthttp.Server, error) {
	h, err := handler.NewMainHandler(cfg)
	if err != nil {
		return nil, err
	}
	s := &fasthttp.Server{
		Handler:      h.Handle,
		ErrorHandler: h.HandleError,
	}
	if err := tree.Unmarshal(cfg.Server, s); err != nil {
		return nil, err
	}
	if cfg.Log.Output != "" {
		s.Logger, err = logger.NewFileLogger(cfg.Log.Output)
		if err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (d *FastHttpd) Run(args []string) error {
	if err := d.initFlagSet(args); err != nil {
		return err
	}
	if d.isHelp || (d.configFile == "" && len(d.editExprs) == 0) {
		d.flagSet.Usage()
		return nil
	}

	cfg, err := d.initConfig()
	if err != nil {
		return err
	}
	s, err := d.newServer(cfg)
	if err != nil {
		return err
	}
	ln, err := net.Listen(getNetwork(cfg.Listen), cfg.Listen)
	if err != nil {
		return err
	}
	if tcpln, ok := ln.(*net.TCPListener); ok {
		return s.Serve(tcpKeepaliveListener{
			TCPListener:     tcpln,
			keepalive:       s.TCPKeepalive,
			keepalivePeriod: s.TCPKeepalivePeriod,
		})
	}
	s.Logger.Printf("Starting HTTP server on %q", cfg.Listen)
	d.server = s
	return s.Serve(ln)
}

func (d *FastHttpd) Shutdown() error {
	return d.server.Shutdown()
}

func RunFastHttpd(args []string) error {
	return NewFastHttpd().Run(args)
}
