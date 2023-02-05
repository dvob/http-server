package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
)

type handlerFactory func(map[string]string) (http.Handler, error)

func noConfigFactory(handler http.HandlerFunc) handlerFactory {
	return func(_ map[string]string) (http.Handler, error) {
		return handler, nil
	}
}

// TODO: use register and move init logic to handler
var handlers = map[string]handlerFactory{
	"info": noConfigFactory(infoHandler),
	"static": func(config map[string]string) (http.Handler, error) {
		handler := newStaticResponseHandler()
		if body, ok := config["body"]; ok {
			handler.body = []byte(body)
		}
		if code, ok := config["code"]; ok {
			num, err := strconv.Atoi(code)
			if err != nil {
				return nil, fmt.Errorf("invalid status code '%s'", code)
			}
			handler.code = num
		}

		return handler, nil
	},
	"echo": noConfigFactory(echoHandler),
	"proxy": func(config map[string]string) (http.Handler, error) {
		target, ok := config["target"]
		if !ok {
			return nil, fmt.Errorf("missing configuration 'target'")
		}

		targetURL, err := url.Parse(target)
		if err != nil {
			return nil, err
		}
		proxy := httputil.NewSingleHostReverseProxy(targetURL)
		return proxy, nil
	},
	"hec":  noConfigFactory(hecHandler),
	"data": noConfigFactory(dataHandler),
	"fs": func(config map[string]string) (http.Handler, error) {
		file, ok := config["file"]
		if !ok {
			return nil, fmt.Errorf("missing configuration 'file'")
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, file)
		}), nil
	},
}

func infoHandler(w http.ResponseWriter, r *http.Request) {
	info := struct {
		Hostname string   `json:"hostname"`
		Request  *request `json:"request"`
	}{}
	info.Hostname, _ = os.Hostname()
	info.Request = newRequest(r)
	err := json.NewEncoder(w).Encode(info)
	if err != nil {
		log.Println("failed to encode json:", err)
	}
}

type staticResponseHandler struct {
	body []byte
	code int
}

func newStaticResponseHandler() *staticResponseHandler {
	return &staticResponseHandler{
		body: []byte("ok\n"),
		code: 200,
	}
}

func (s *staticResponseHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(s.code)
	w.Write(s.body)
}

func echoHandler(w http.ResponseWriter, r *http.Request) {
	io.Copy(w, r.Body)
}

func hecHandler(w http.ResponseWriter, r *http.Request) {
	scanner := bufio.NewScanner(r.Body)
	for scanner.Scan() {
		var payload any
		err := json.Unmarshal([]byte(scanner.Text()), &payload)
		if err != nil {
			log.Print("failed to parse event")
			continue
		}
		out, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			panic(err)
		}
		fmt.Println(string(out))
	}

	if err := scanner.Err(); err != nil {
		log.Print(err)
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

	_, err = io.Copy(w, newNBytesReader(size))
	if err != nil {
		log.Print(err)
	}
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

type request struct {
	Method     string      `json:"method"`
	URI        string      `json:"uri"`
	Protocol   string      `json:"protocol"`
	Header     http.Header `json:"header"`
	RemoteAddr string      `json:"remote_addr"`
	// TLS evtl.
}

func newRequest(r *http.Request) *request {
	return &request{
		Method:     r.Method,
		URI:        r.RequestURI,
		Protocol:   r.Proto,
		Header:     r.Header,
		RemoteAddr: r.RemoteAddr,
	}
}
