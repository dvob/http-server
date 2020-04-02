package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
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
	Stdout      io.Writer
	Stderr      io.Writer
	ShowHeaders bool
}

func RawHandler(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handleHeader(cfg, r)

		_, err := io.Copy(cfg.Stdout, r.Body)
		if err != nil {
			fmt.Fprintln(cfg.Stderr, err)
		}

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

func handleJSON() {
}
