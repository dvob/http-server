package main

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
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
	middleware        string
	handler           string
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

func run() error {
	// fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	// fs.Usage = func() {
	// 	// TODO: extend with description of handlers and middlewares
	// 	fs.PrintDefaults()
	// }
	// fs.Parse(os.Args[1:])

	serverConfig := newDefaultServer()
	serverConfig.bindFlags(flag.CommandLine)
	flag.Parse()

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

func old_main() {
	var (
		jsonMode   bool
		hecMode    bool
		enableTLS  bool
		tlsCert    string
		tlsKey     string
		listenAddr string
	)

	cfg := Config{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}

	flag.BoolVar(&jsonMode, "json", false, "Parse body as json")
	flag.BoolVar(&enableTLS, "tls", false, "Enable TLS")
	flag.BoolVar(&cfg.ShowHeaders, "header", true, "show headers")
	flag.BoolVar(&hecMode, "hec", false, "HTTP Event Collector Mode")
	flag.BoolVar(&cfg.Info, "info", false, "Return request info as content")
	flag.BoolVar(&cfg.EnableTimeout, "timeout", false, "enable timeout endpoint (e.g. */timeout?duration=12s)")
	flag.BoolVar(&cfg.EnableData, "data", false, "Enable data endpoint (e.g */data?size=123)")
	flag.StringVar(&cfg.Content, "content", "", "Body which gets returned")
	flag.StringVar(&tlsCert, "cert", "tls.crt", "TLS certificate")
	flag.StringVar(&tlsKey, "key", "tls.key", "TLS key")
	flag.StringVar(&listenAddr, "addr", ":8080", "Listen address")
	flag.Parse()
}

type Config struct {
	Stdout        io.Writer
	Stderr        io.Writer
	Content       string
	Info          bool
	ShowHeaders   bool
	EnableTimeout bool
	EnableData    bool
}

func RawHandler(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		if cfg.EnableTimeout && strings.HasSuffix(r.URL.Path, "/timeout") {
			return
		}
		if cfg.EnableData && strings.HasSuffix(r.URL.Path, "/data") {
			dataHandler(w, r)
			return
		}

		_, err := io.Copy(cfg.Stdout, r.Body)
		if err != nil {
			fmt.Fprintln(cfg.Stderr, err)
		}

		return
	}
}

func HECHandler(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scanner := bufio.NewScanner(r.Body)
		for scanner.Scan() {
			payload := make(map[string]interface{})
			err := json.Unmarshal([]byte(scanner.Text()), &payload)
			if err != nil {
				fmt.Fprintln(cfg.Stderr, "failed to parse event:", err)
				continue
			}
			enc := json.NewEncoder(cfg.Stdout)
			enc.SetIndent("", "  ")
			err = enc.Encode(payload)
			if err != nil {
				fmt.Fprintln(cfg.Stderr, err)
				return
			}
		}

		if err := scanner.Err(); err != nil {
			fmt.Fprintln(cfg.Stderr, err)
		}
		return
	}
}

func dataHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	sizeStr := r.URL.Query().Get("size")
	size := 0
	if sizeStr != "" {
		size, err = strconv.Atoi(sizeStr)
		if err != nil {
			http.Error(w, "invalid size: "+err.Error(), 400)
			return
		}
	}

	switch r.Method {
	case "POST":
		received, err := io.Copy(os.Stdout, r.Body)
		if err != nil {
			log.Printf("failed to read request body: %s", err)
			http.Error(w, "failed to read body: "+err.Error(), 400)
		}
		fmt.Fprintln(w, strconv.FormatInt(received, 10))
		return
	case "GET":
		_, err := io.Copy(w, newNBytesReader(size))
		if err != nil {
			http.Error(w, "failed to send data", 500)
			return
		}
	default:
		http.Error(w, "method not allowed", 405)
		return
	}
	return
}

func newNBytesReader(size int) *nBytesReader {
	return &nBytesReader{
		n: size,
	}
}

type nBytesReader struct {
	// total bytes to return
	n int
	// already returned bytes
	sent int
}

func (n *nBytesReader) Read(p []byte) (int, error) {
	sent := 0
	for ; sent < len(p) && n.sent < n.n; sent++ {
		p[sent] = 'A'
		n.sent++
	}
	if n.sent == n.n {
		return sent, io.EOF
	}
	return sent, nil
}
