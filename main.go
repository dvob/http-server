package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dvob/http-server/config"
	"golang.org/x/crypto/acme/autocert"
)

type serverConfig struct {
	addr              string
	readTimeout       time.Duration
	readHeaderTimeout time.Duration
	writeTimeout      time.Duration
	idleTimeout       time.Duration
	maxHeaderBytes    int
	tlsConfig         tlsConfig
	connLog           bool
}

func newDefaultServer() serverConfig {
	return serverConfig{
		tlsConfig: newDefaultTLSConfig(),
	}
}

func (s *serverConfig) bindFlags(fs *flag.FlagSet) {
	fs.StringVar(&s.addr, "addr", s.addr, "listen address. if tls is configured this defaults to ':443' otherwise to ':80'")
	fs.DurationVar(&s.readTimeout, "read-timeout", s.readTimeout, "read timeout")
	fs.DurationVar(&s.readHeaderTimeout, "read-header-timeout", s.readHeaderTimeout, "read header timeout")
	fs.DurationVar(&s.writeTimeout, "write-timeout", s.writeTimeout, "write timeout")
	fs.DurationVar(&s.idleTimeout, "idle-timeout", s.idleTimeout, "idle timeout")
	fs.BoolVar(&s.connLog, "conn-log", s.connLog, "enable connection log")
	s.tlsConfig.bindFlags(fs)
}

func (s *serverConfig) getServer() (*http.Server, error) {
	tlsConfig, err := s.tlsConfig.getConfig()
	if err != nil {
		return nil, err
	}

	// set default addr
	if s.addr == "" {
		if tlsConfig == nil {
			s.addr = ":80"
		} else {
			s.addr = ":443"
		}
	}

	var connStateFn func(net.Conn, http.ConnState)
	if s.connLog {
		connStateFn = func(c net.Conn, s http.ConnState) {
			if s == http.StateIdle || s == http.StateActive {
				return
			}
			log.Printf("%s %s", s, c.RemoteAddr())
			//if s == http.StateNew {
			//	tcpConn, ok := c.(*net.TCPConn)
			//	if ok {
			//		tcpConn.SetKeepAlive(false)
			//	}
			//}
		}
	}

	srv := &http.Server{
		Addr:              s.addr,
		TLSConfig:         tlsConfig,
		ReadTimeout:       s.readTimeout,
		ReadHeaderTimeout: s.readHeaderTimeout,
		WriteTimeout:      s.writeTimeout,
		IdleTimeout:       s.idleTimeout,
		MaxHeaderBytes:    s.maxHeaderBytes,
		ConnState:         connStateFn,
	}
	return srv, nil
}

func (s *serverConfig) run(handler http.Handler) error {
	srv, err := s.getServer()
	if err != nil {
		return err
	}

	srv.Handler = handler

	if srv.TLSConfig == nil {
		return srv.ListenAndServe()
	} else {
		// certificates are explicitly configured in the TLSConfig
		return srv.ListenAndServeTLS("", "")
	}
}

type tlsConfig struct {
	cert     string
	key      string
	hosts    string
	cacheDir string
}

func newDefaultTLSConfig() tlsConfig {
	return tlsConfig{
		cacheDir: "cert-dir",
	}
}

func (t *tlsConfig) bindFlags(fs *flag.FlagSet) {
	fs.StringVar(&t.cert, "tls-cert", t.cert, "path to PEM encodeded certificate")
	fs.StringVar(&t.key, "tls-key", t.key, "path to PEM encodeded key")
	fs.StringVar(&t.hosts, "tls-hosts", t.hosts, "enables automatic certificate management with ACME (Let's Encrypt) for the specified list of comma-seperated hostnames")
	fs.StringVar(&t.cacheDir, "tls-cache-dir", t.cacheDir, "cache dir for ACME certificates")
}

func (t *tlsConfig) getConfig() (*tls.Config, error) {
	// ACME (Let's Encrypt)
	if t.hosts != "" {
		hosts := strings.Split(t.hosts, ",")
		manager := autocert.Manager{
			Cache:      autocert.DirCache(t.cacheDir),
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(hosts...),
		}
		return manager.TLSConfig(), nil
	}

	// Local Certificate File
	if t.cert != "" || t.key != "" {
		cert, err := tls.LoadX509KeyPair(t.cert, t.key)
		if err != nil {
			return nil, err
		}
		return &tls.Config{
			Certificates: []tls.Certificate{cert},
		}, nil
	}

	// TLS disabled
	return nil, nil
}

func buildHanlderChain(cfgChain []config.HandlerConfig) (http.Handler, error) {
	if len(cfgChain) == 0 {
		return logRequest(newStaticResponseHandler().ServeHTTP), nil
	}
	mws := []middleware{}
	for _, mw := range cfgChain[:len(cfgChain)-1] {
		middlewareHandlerFactory, ok := middlewares[mw.Name]
		if !ok {
			return nil, fmt.Errorf("could not find middleware: %s", mw.Name)
		}
		middlewareHandler, err := middlewareHandlerFactory(mw.Settings)
		if err != nil {
			return nil, fmt.Errorf("failed to configure middleware %s: %w", mw.Name, err)
		}
		mws = append(mws, middlewareHandler)
	}
	handlerCfg := cfgChain[len(cfgChain)-1]
	handlerFactory, ok := handlers[handlerCfg.Name]
	if !ok {
		return nil, fmt.Errorf("handler %s not found", handlerCfg.Name)
	}
	handler, err := handlerFactory(handlerCfg.Settings)
	if err != nil {
		return nil, fmt.Errorf("invalid configuration in '%s' handler: %w", handlerCfg.Name, err)
	}
	return chain(mws...)(handler.ServeHTTP), nil
}

func getHandler(cfg map[string][]config.HandlerConfig) (http.Handler, error) {
	if len(cfg) == 0 {
		return logRequest(newStaticResponseHandler().ServeHTTP), nil
	}

	// we don't use a mux if there is only the root
	if chain, ok := cfg["/"]; len(cfg) == 1 && ok {
		return buildHanlderChain(chain)
	}

	mux := http.NewServeMux()
	for path, chain := range cfg {
		handler, err := buildHanlderChain(chain)
		if err != nil {
			return nil, err
		}
		mux.Handle(path, handler)
	}
	return mux, nil
}

func listOptions() {
	fmt.Println("handlers:")
	for handler := range handlers {
		fmt.Println(handler)
	}
	fmt.Println()

	fmt.Println("middlewares:")
	for middleware := range middlewares {
		fmt.Println(middleware)
	}
}

func run() error {
	// list handlers and middlewares
	var list bool
	// fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	// fs.Usage = func() {
	// 	// TODO: extend with description of handlers and middlewares
	// 	fs.PrintDefaults()
	// }
	// fs.Parse(os.Args[1:])

	serverConfig := newDefaultServer()
	serverConfig.bindFlags(flag.CommandLine)
	flag.BoolVar(&list, "list", false, "list available handlers and middlewares")
	flag.Parse()

	if list {
		listOptions()
		return nil
	}

	cfg, err := config.ParseArgs(flag.Args())
	if err != nil {
		return err
	}

	handler, err := getHandler(cfg)
	if err != nil {
		return err
	}

	err = serverConfig.run(handler)
	if err != nil {
		return err
	}
	return nil
}

func main() {
	err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
