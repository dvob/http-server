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
	fs.StringVar(&s.addr, "addr", s.addr, "listen addr. if tls configures this defaults to ':443' otherwise ':80'")
	fs.DurationVar(&s.readTimeout, "read-timeout", s.readTimeout, "read timeout")
	fs.DurationVar(&s.readHeaderTimeout, "read-header-timeout", s.readHeaderTimeout, "read header timeout")
	fs.DurationVar(&s.writeTimeout, "write-timeout", s.writeTimeout, "write timeout")
	fs.DurationVar(&s.idleTimeout, "idle-timeout", s.idleTimeout, "idle timeout")
	fs.BoolVar(&s.connLog, "conn-log", s.connLog, "enable connection log")
	fs.StringVar(&s.handler, "handler", s.handler, "handler")
	fs.StringVar(&s.middleware, "middleware", s.middleware, "middleware")
	s.tlsConfig.bindFlags(fs)
}

func (s *serverConfig) getHandler() (http.Handler, error) {
	var (
		handler         http.HandlerFunc
		middlewareChain []middleware
	)

	handler = okHandler

	if s.middleware == "" {
		// set default middleware
		middlewareChain = []middleware{
			dumpRequest,
		}
	} else {
		for _, middlewareName := range strings.Split(s.middleware, ",") {
			middleware, ok := middlewares[middlewareName]
			if !ok {
				return nil, fmt.Errorf("middleware '%s' does not exist", middlewareName)
			}
			middlewareChain = append(middlewareChain, middleware)
		}
	}

	if s.handler != "" {
		var ok bool
		handler, ok = handlers[s.handler]
		if !ok {
			return nil, fmt.Errorf("handler '%s' does not exist", s.handler)
		}
	}

	return chain(middlewareChain...)(handler), nil
}

func (s *serverConfig) getServer() (*http.Server, error) {
	tlsConfig, err := s.tlsConfig.getConfig()
	if err != nil {
		return nil, err
	}

	handler, err := s.getHandler()
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
		Handler:           handler,
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

func (s *serverConfig) run() error {
	srv, err := s.getServer()
	if err != nil {
		return err
	}
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

func run() error {
	serverConfig := newDefaultServer()
	serverConfig.bindFlags(flag.CommandLine)
	flag.Parse()

	err := serverConfig.run()
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
