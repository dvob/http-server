package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/felixge/httpsnoop"
)

var middlewares = map[string]middleware{
	"timeout": timeout,
	"req":     dumpRequest,
	"log":     logRequest,
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

func logRequest(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := httpsnoop.CaptureMetrics(next, w, r)
		log.Printf(
			"src=%s method=%s url=%s code=%d dt=%s written=%d",
			r.RemoteAddr,
			r.Method,
			r.URL,
			m.Code,
			m.Duration,
			m.Written,
		)
	}
}
func dumpRequest(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, _ := httputil.DumpRequest(r, false)
		log.Print(string(req))
		next(w, r)
	}
}

func jsonLogger(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const maxSize = 1_000_000 // 1MB
		// -1 means chunked
		if r.ContentLength <= 0 || r.ContentLength > maxSize {
			next(w, r)
			return
		}

		buf := &bytes.Buffer{}
		_, err := buf.ReadFrom(io.LimitReader(r.Body, maxSize))
		if err != nil {
			log.Print(err)
			return
		}
		dst := &bytes.Buffer{}
		err = json.Indent(dst, buf.Bytes(), "", "  ")
		if err != nil {
			log.Print("could not print json:", err)
		} else {
			fmt.Println(dst.String())
		}
		r.Body = io.NopCloser(buf)
		next(w, r)
	}
}
