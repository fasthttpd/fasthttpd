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
	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
)

const (
	cmd          = "fasthttpd"
	version      = "0.0.2"
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
		Log:  config.Log{Output: "stderr"}.SetDefaults(),
		Handlers: map[string]tree.Map{
			"static": {
				"type":       tree.ToValue("fs"),
				"indexNames": tree.ToArrayValues("index.html"),
			},
		},
		Routes: []config.Route{{Handler: "static"}},
	}.SetDefaults().Normalize()
}

type FastHttpd struct {
	flagSet    *flag.FlagSet
	isVersion  bool
	isHelp     bool
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
		n, err := tree.MarshalViaYAML(cfg)
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
		if err := tree.UnmarshalViaYAML(n, &cfg); err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}

func (d *FastHttpd) newServer(cfg config.Config, h *handler.MainHandler) (*fasthttp.Server, error) {
	s := &fasthttp.Server{
		Handler:      h.Handle,
		ErrorHandler: h.HandleError,
		Logger:       h.Logger(),
	}
	if err := tree.UnmarshalViaJSON(cfg.Server, s); err != nil {
		return nil, err
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
	if d.isVersion {
		fmt.Println(version)
		return nil
	}

	cfg, err := d.initConfig()
	if err != nil {
		return err
	}
	h, err := handler.NewMainHandler(cfg)
	if err != nil {
		return err
	}
	defer h.Close()

	s, err := d.newServer(cfg, h)
	if err != nil {
		return err
	}
	ln, err := netListen(cfg.Listen)
	if err != nil {
		return err
	}
	s.Logger.Printf("Starting HTTP server on %q", cfg.Listen)
	d.server = s

	if tcpln, ok := ln.(*net.TCPListener); ok {
		return s.Serve(tcpKeepaliveListener{
			TCPListener:     tcpln,
			keepalive:       s.TCPKeepalive,
			keepalivePeriod: s.TCPKeepalivePeriod,
		})
	}
	return s.Serve(ln)
}

func (d *FastHttpd) Shutdown() error {
	if d.server != nil {
		return d.server.Shutdown()
	}
	return nil
}

func RunFastHttpd(args []string) error {
	return NewFastHttpd().Run(args)
}
