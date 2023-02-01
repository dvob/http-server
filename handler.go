package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
)

var handlers = map[string]http.HandlerFunc{
	"info": infoHandler,
	"ok":   (&staticResponseHandler{}).ServeHTTP,
	"echo": echoHandler,
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
	body   []byte
	code   int
	header http.Header
}

func (s *staticResponseHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	appendHeader(w.Header(), s.header)
	w.WriteHeader(s.code)
	w.Write(s.body)
}

func echoHandler(w http.ResponseWriter, r *http.Request) {
	io.Copy(w, r.Body)
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

func appendHeader(dst http.Header, src http.Header) {
	for header, values := range src {
		for _, value := range values {
			dst.Add(header, value)
		}
	}
}
