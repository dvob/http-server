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
	"ok":   okHandler,
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

func okHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok\n"))
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
