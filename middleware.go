package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"time"
)

var middlewares = map[string]middleware{
	"timeout": timeout,
	"log":     requestLogger,
	"json":    jsonLogger,
}

type middleware func(http.HandlerFunc) http.HandlerFunc

func chain(middlewares ...middleware) middleware {
	return func(h http.HandlerFunc) http.HandlerFunc {
		for i := range middlewares {
			h = middlewares[len(middlewares)-1-i](h)
		}
		return h
	}
}

func timeout(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		params := r.URL.Query()
		if params.Has("duration") {
			rawDuration := params.Get("duration")
			duration, err := time.ParseDuration(rawDuration)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			time.Sleep(duration)
		}
		next(w, r)
	}
}

func requestLogger(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, _ := httputil.DumpRequest(r, false)
		log.Print(string(req))
		next(w, r)
	}
}

func jsonLogger(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		buf := &bytes.Buffer{}
		_, err := buf.ReadFrom(r.Body)
		if err != nil {
			log.Print(err)
		}
		dst := &bytes.Buffer{}
		err = json.Indent(dst, buf.Bytes(), "", "  ")
		if err != nil {
			log.Print(dst.String())
		} else {
			log.Print("could not print json:", err)
		}
		r.Body = io.NopCloser(buf)
		next(w, r)
	}
}