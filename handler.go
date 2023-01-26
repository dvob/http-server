package main

import (
	"encoding/json"
	"io"
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
		Hostname string `json:"hostname"`
	}{}
	info.Hostname, _ = os.Hostname()
	json.NewEncoder(w).Encode(info)
}

func okHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok\n"))
}

func echoHandler(w http.ResponseWriter, r *http.Request) {
	io.Copy(w, r.Body)
}
