package cmd

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/handler"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
)

// EnvFasthttpdConfig is the environment variable name "FASTHTTPD_CONFIG" that
// indicates the default configuration file path.
const EnvFasthttpdConfig = "FASTHTTPD_CONFIG"

const (
	cmd          = "fasthttpd"
	version      = "0.3.2"
	desc         = "FastHttpd is a HTTP server using valyala/fasthttp."
	usage        = cmd + " [flags]"
	examplesText = `Examples:
  % fasthttpd -f ./examples/config.minimal.yaml
  % fasthttpd -e root=./examples/public -e listen=:8080
`
)

func newMinimalConfig() config.Config {
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
	}.SetDefaults()
}

type FastHttpd struct {
	flagSet    *flag.FlagSet
	isVersion  bool
	isHelp     bool
	configFile string
	editExprs  stringList
	servers    []*fasthttp.Server
	stopHup    context.CancelFunc
}

func NewFastHttpd() *FastHttpd {
	return &FastHttpd{}
}

func (d *FastHttpd) initFlagSet(args []string) error {
	s := flag.NewFlagSet(args[0], flag.ExitOnError)
	d.flagSet = s

	s.BoolVar(&d.isVersion, "v", false, "print version")
	s.BoolVar(&d.isHelp, "h", false, "help for "+cmd)
	s.StringVar(&d.configFile, "f", os.Getenv(EnvFasthttpdConfig), "configuration file")
	s.Var(&d.editExprs, "e", "edit expression (eg. -e KEY=VALUE)")
	s.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s\n\nUsage:\n  %s\n\n", desc, usage)
		fmt.Fprintln(os.Stderr, "Flags:")
		s.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n%s", examplesText)
	}
	return s.Parse(args[1:])
}

func (d *FastHttpd) loadConfigs() (map[string][]config.Config, error) {
	if d.configFile == "" {
		return d.editConfigs([]config.Config{newMinimalConfig()})
	}
	cfgs, err := config.UnmarshalYAMLPath(d.configFile)
	if err != nil {
		return nil, err
	}
	if err := os.Chdir(filepath.Dir(d.configFile)); err != nil {
		return nil, err
	}
	return d.editConfigs(cfgs)
}

func (d *FastHttpd) editConfigs(cfgs []config.Config) (map[string][]config.Config, error) {
	if len(d.editExprs) > 0 {
		n, err := tree.MarshalViaYAML(cfgs)
		if err != nil {
			return nil, err
		}
		for _, expr := range d.editExprs {
			lr := strings.SplitN(expr, "=", 2)
			if len(lr) == 2 {
				if !strings.HasPrefix(lr[0], ".") {
					lr[0] = ".[]." + lr[0]
				}
				if _, err := strconv.Atoi(lr[1]); err != nil && !strings.HasPrefix(lr[1], `"`) {
					lr[1] = strconv.Quote(lr[1])
				}
				expr = lr[0] + "=" + lr[1]
			}
			if err := tree.Edit(&n, expr); err != nil {
				return nil, err
			}
		}
		if err := tree.UnmarshalViaYAML(n, &cfgs); err != nil {
			return nil, err
		}
	}
	m := map[string][]config.Config{}
	for _, cfg := range cfgs {
		m[cfg.Listen] = append(m[cfg.Listen], cfg)
	}
	return m, nil
}

func (d *FastHttpd) newServer(h handler.ServerHandler) (*fasthttp.Server, error) {
	s := &fasthttp.Server{
		Handler:      h.Handle,
		ErrorHandler: h.HandleError,
		Logger:       h.Logger(),
	}
	if err := tree.UnmarshalViaJSON(h.Config(), s); err != nil {
		return nil, err
	}
	return s, nil
}

func (d *FastHttpd) listen(cfgs []config.Config, server *fasthttp.Server) (net.Listener, error) {
	ln, err := netListen(cfgs[0].Listen)
	if err != nil {
		return nil, err
	}
	if tcpln, ok := ln.(*net.TCPListener); ok {
		ln = &tcpKeepaliveListener{
			TCPListener:     tcpln,
			keepalive:       server.TCPKeepalive,
			keepalivePeriod: server.TCPKeepalivePeriod,
		}
	}
	tlsCfg, err := tlsConfig(cfgs)
	if err != nil {
		return nil, err
	}
	if tlsCfg != nil {
		ln = tls.NewListener(ln, tlsCfg)
	}
	return ln, nil
}

func (d *FastHttpd) run() error {
	lcfgs, err := d.loadConfigs()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT)
	defer stop()
	go func() {
		<-ctx.Done()
		// log.Printf("signal int: shutdown fasthttpd")
		if err := d.Shutdown(); err != nil {
			log.Printf("failed to shutdown: %v", err)
		}
	}()

	d.handleHUP()

	errChs := make(chan error, 2)
	for l, cfgs := range lcfgs {
		h, err := handler.NewServerHandler(cfgs)
		if err != nil {
			return err
		}
		defer h.Close()

		server, err := d.newServer(h)
		if err != nil {
			return err
		}
		ln, err := d.listen(cfgs, server)
		if err != nil {
			return err
		}

		h.Logger().Printf("starting fasthttpd on %q", l)
		d.servers = append(d.servers, server)

		go func() {
			err := server.Serve(ln)
			errChs <- err
		}()
	}

	var errs []string
	for range lcfgs {
		if err := <-errChs; err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to serve: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (d *FastHttpd) handleHUP() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGHUP)
	d.stopHup = stop
	go func() {
		for {
			<-ctx.Done()
			if d.stopHup == nil {
				break
			}
			// log.Printf("signal hup: rotate logs")
			if err := logger.RotateShared(); err != nil {
				log.Printf("failed to rotate stored: %v", err)
			}
			stop()
			ctx, stop = signal.NotifyContext(context.Background(), syscall.SIGHUP)
			d.stopHup = stop
		}
	}()
}

func (d *FastHttpd) Shutdown() error {
	if stopHup := d.stopHup; stopHup != nil {
		d.stopHup = nil
		stopHup()
	}
	var errs []string
	for _, server := range d.servers {
		if err := server.Shutdown(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf(strings.Join(errs, "; "))
}

func (d *FastHttpd) Main(args []string) error {
	if err := d.initFlagSet(args); err != nil {
		return err
	}
	if d.isVersion {
		fmt.Println(version)
		return nil
	}
	if d.isHelp || (d.configFile == "" && len(d.editExprs) == 0) {
		d.flagSet.Usage()
		return nil
	}

	return d.run()
}

func RunFastHttpd(args []string) error {
	return NewFastHttpd().Main(args)
}
