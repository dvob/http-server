package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
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

	if jsonMode {
		http.HandleFunc("/", JSONHandler(cfg))
	} else if hecMode {
		http.HandleFunc("/", HECHandler(cfg))
	} else {
		http.HandleFunc("/", RawHandler(cfg))
	}

	if enableTLS {
		log.Fatal(http.ListenAndServeTLS(listenAddr, tlsCert, tlsKey, nil))
	} else {
		log.Fatal(http.ListenAndServe(listenAddr, nil))
	}
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
		handleHeader(cfg, r)

		if cfg.EnableTimeout && strings.HasSuffix(r.URL.Path, "/timeout") {
			timeoutHandler(w, r)
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

		writeContent(cfg, r, w)

		return
	}
}

func JSONHandler(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handleHeader(cfg, r)

		payload := make(map[string]interface{})
		err := json.NewDecoder(r.Body).Decode(&payload)
		if err != nil {
			fmt.Fprintln(cfg.Stderr, err)
			return
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		err = enc.Encode(payload)
		if err != nil {
			fmt.Fprintln(cfg.Stderr, err)
			return
		}

		writeContent(cfg, r, w)

	}

}

func HECHandler(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handleHeader(cfg, r)

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

func handleHeader(cfg Config, r *http.Request) {
	if !cfg.ShowHeaders {
		return
	}
	header, err := httputil.DumpRequest(r, false)
	if err != nil {
		fmt.Fprintln(cfg.Stderr, err)
		return
	}
	fmt.Fprintln(cfg.Stdout, string(header))
}

func timeoutHandler(w http.ResponseWriter, r *http.Request) {
	duration, err := time.ParseDuration(r.URL.Query().Get("duration"))
	if err != nil {
		http.Error(w, "failed to parse duration: "+err.Error(), 400)
		return
	}
	time.Sleep(duration)
	return
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

func writeContent(cfg Config, r *http.Request, w http.ResponseWriter) {
	content := ""

	if cfg.Content != "" {
		content = cfg.Content + "\n"
	}

	if cfg.Info {
		content = content + getInfo(r)
	}

	if content != "" {
		fmt.Fprintf(w, content)
	}
}

func getInfo(r *http.Request) string {
	info := &bytes.Buffer{}
	hostname, _ := os.Hostname()
	fmt.Fprintf(info, "hostname: '%s'\n", hostname)

	header, err := httputil.DumpRequest(r, false)
	if err != nil {
		header = []byte{}
	}
	fmt.Fprintf(info, "http request:\n")
	info.Write(header)
	fmt.Fprintf(info, "---\n")

	return info.String()
}
