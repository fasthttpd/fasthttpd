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
	fasthttpdnet "github.com/fasthttpd/fasthttpd/pkg/net"
	"github.com/fasthttpd/fasthttpd/pkg/util"
	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
)

// EnvFasthttpdConfig is the environment variable name "FASTHTTPD_CONFIG" that
// indicates the default configuration file path.
const EnvFasthttpdConfig = "FASTHTTPD_CONFIG"

const (
	cmd          = "fasthttpd"
	version      = "0.4.0"
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
		Log:  config.Log{Output: "stderr"},
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
	editExprs  util.StringList
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

func (d *FastHttpd) loadConfigs() ([]config.Config, error) {
	if d.configFile == "" {
		return []config.Config{newMinimalConfig()}, nil
	}
	dir, file := filepath.Split(d.configFile)
	if dir != "" {
		if err := os.Chdir(dir); err != nil {
			return nil, err
		}
	}
	cfgs, err := config.UnmarshalYAMLPath(file)
	if err != nil {
		return nil, err
	}
	return cfgs, nil
}

func (d *FastHttpd) editConfigs(cfgs []config.Config) ([]config.Config, error) {
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
				if !strings.HasPrefix(lr[1], `"`) &&
					lr[1] != "true" && lr[1] != "false" && lr[1] != "null" {
					if _, err := strconv.Atoi(lr[1]); err != nil {
						lr[1] = strconv.Quote(lr[1])
					}
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
	return cfgs, nil
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

var netListen = func(listen string) (net.Listener, error) {
	return net.Listen(fasthttpdnet.GetNetwork(listen), listen)
}

func (d *FastHttpd) listen(listen string, cfgs []config.Config, server *fasthttp.Server) (net.Listener, error) {
	ln, err := netListen(listen)
	if err != nil {
		return nil, err
	}
	if tcpln, ok := ln.(*net.TCPListener); ok {
		ln = &fasthttpdnet.TcpKeepaliveListener{
			TCPListener:     tcpln,
			Keepalive:       server.TCPKeepalive,
			KeepalivePeriod: server.TCPKeepalivePeriod,
		}
	}
	tlsCfg, err := fasthttpdnet.MultiTLSConfig(cfgs)
	if err != nil {
		return nil, err
	}
	if tlsCfg != nil {
		ln = tls.NewListener(ln, tlsCfg)
	}
	return ln, nil
}

func (d *FastHttpd) run() error {
	cfgs, err := d.loadConfigs()
	if err != nil {
		return err
	}
	cfgs, err = d.editConfigs(cfgs)
	if err != nil {
		return err
	}
	listenedCfgs := map[string][]config.Config{}
	for _, cfg := range cfgs {
		listenedCfgs[cfg.Listen] = append(listenedCfgs[cfg.Listen], cfg)
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
	for listen, cfgs := range listenedCfgs {
		h, err := handler.NewServerHandler(cfgs)
		if err != nil {
			return err
		}
		defer h.Close()

		server, err := d.newServer(h)
		if err != nil {
			return err
		}
		ln, err := d.listen(listen, cfgs, server)
		if err != nil {
			return err
		}

		h.Logger().Printf("starting fasthttpd on %q", listen)
		d.servers = append(d.servers, server)

		go func() {
			err := server.Serve(ln)
			errChs <- err
		}()
	}

	var errs []string
	for range listenedCfgs {
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
