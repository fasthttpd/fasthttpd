package cmd

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/handler"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	fasthttpdnet "github.com/fasthttpd/fasthttpd/pkg/net"
	"github.com/fasthttpd/fasthttpd/pkg/util"
	"github.com/mojatter/tree"
	"github.com/valyala/fasthttp"
)

// EnvFasthttpdConfig is the environment variable name "FASTHTTPD_CONFIG" that
// indicates the default configuration file path.
const EnvFasthttpdConfig = "FASTHTTPD_CONFIG"

// version is injected at build time via -ldflags "-X ...version=...".
// Defaults to "dev" for `go install` / `go run` style builds.
var version = "dev"

const (
	cmd          = "fasthttpd"
	desc         = "FastHttpd is a lightweight http server using valyala/fasthttp."
	usage        = cmd + " [flags]"
	examplesText = `Examples:
  % fasthttpd -f ./examples/config.minimal.yaml
  % fasthttpd -e root=./examples/public -e listen=:8080
`
)

// minimalTreeMap returns the fallback configuration used when no -f
// file is supplied but at least one -e expression is given.
func minimalTreeMap() tree.Map {
	return tree.Map{
		"host": tree.V("localhost"),
		"root": tree.V("./public"),
		"log":  tree.Map{"output": tree.V("stderr")},
		"handlers": tree.Map{
			"static": tree.Map{
				"type":       tree.V("fs"),
				"indexNames": tree.A("index.html"),
			},
		},
		"routes": tree.Array{tree.Map{"handler": tree.V("static")}},
	}
}

type FastHttpd struct {
	flagSet          *flag.FlagSet
	isVersion        bool
	isHelp           bool
	configFile       string
	editExprs        util.StringList
	servers          []*fasthttp.Server
	shutdownTimeouts []time.Duration

	hupMu    sync.Mutex
	hupCh    chan os.Signal
	hupClose sync.Once
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

func (d *FastHttpd) loadTreeMaps() ([]tree.Map, error) {
	if d.configFile == "" {
		return []tree.Map{minimalTreeMap()}, nil
	}
	dir, file := filepath.Split(d.configFile)
	if dir != "" {
		if err := os.Chdir(dir); err != nil {
			return nil, err
		}
	}
	return config.LoadTreeMaps(file)
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
	ms, err := d.loadTreeMaps()
	if err != nil {
		return err
	}
	ms, err = config.Edit(ms, d.editExprs)
	if err != nil {
		return err
	}
	cfgs, err := config.FromTreeMaps(ms)
	if err != nil {
		return err
	}
	listenedCfgs := map[string][]config.Config{}
	for _, cfg := range cfgs {
		listenedCfgs[cfg.Listen] = append(listenedCfgs[cfg.Listen], cfg)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		if err := d.Shutdown(); err != nil {
			log.Printf("failed to shutdown: %v", err)
		}
	}()

	d.handleHUP()

	errChs := make(chan error, len(listenedCfgs))
	for listen, cfgs := range listenedCfgs {
		h, err := handler.NewServerHandler(cfgs)
		if err != nil {
			return err
		}
		defer func() { _ = h.Close() }()

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
		d.shutdownTimeouts = append(d.shutdownTimeouts, cfgs[0].ShutdownTimeoutDuration())

		go func() {
			err := server.Serve(ln)
			errChs <- err
		}()
	}

	var errs []error
	for range listenedCfgs {
		if err := <-errChs; err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to serve: %v", errors.Join(errs...))
	}
	return nil
}

func (d *FastHttpd) handleHUP() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP)

	d.hupMu.Lock()
	d.hupCh = ch
	d.hupMu.Unlock()

	go func() {
		for range ch {
			log.Println("received SIGHUP, rotating logs")
			if err := logger.RotateShared(); err != nil {
				log.Printf("failed to rotate logs: %v", err)
			}
		}
	}()
}

func (d *FastHttpd) stopHUP() {
	d.hupMu.Lock()
	ch := d.hupCh
	d.hupMu.Unlock()

	if ch == nil {
		return
	}

	d.hupClose.Do(func() {
		signal.Stop(ch)
		close(ch)
	})
}

func (d *FastHttpd) Shutdown() error {
	d.stopHUP()

	var wg sync.WaitGroup
	errs := make([]error, len(d.servers))
	for i, server := range d.servers {
		wg.Go(func() {
			ctx := context.Background()
			if timeout := d.shutdownTimeouts[i]; timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}
			if err := server.ShutdownWithContext(ctx); err != nil {
				errs[i] = err
			}
		})
	}
	wg.Wait()
	return errors.Join(errs...)
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
