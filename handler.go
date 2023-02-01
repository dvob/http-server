package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
)

type handlerFactory func(map[string]string) (http.Handler, error)

func noConfigFactory(handler http.HandlerFunc) handlerFactory {
	return func(_ map[string]string) (http.Handler, error) {
		return handler, nil
	}
}

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
