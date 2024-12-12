package main

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
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

		rewriteFunc := func(pr *httputil.ProxyRequest) {
			pr.SetURL(targetURL)
			pr.SetXForwarded()
			// pr.Out.Host = pr.In.Host
		}

		// prepare reverse proxy for HTTP/1.1
		http11Transport := http.DefaultTransport.(*http.Transport).Clone()
		http11Transport.ForceAttemptHTTP2 = false
		http11Transport.TLSNextProto = make(map[string]func(authority string, c *tls.Conn) http.RoundTripper)
		http11Transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}

		http11Upstream := &httputil.ReverseProxy{
			Rewrite:   rewriteFunc,
			Transport: http11Transport,
		}

		// prepare default reverse proxy which uses HTTP/2 if the upstream supports it
		defaultTransport := http.DefaultTransport.(*http.Transport).Clone()
		defaultTransport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
		defaultUpstream := &httputil.ReverseProxy{
			Rewrite:   rewriteFunc,
			Transport: defaultTransport,
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Upgrade is only supported by HTTP/1.1
			if r.Proto == "HTTP/1.1" && r.Header.Get("Upgrade") != "" {
				http11Upstream.ServeHTTP(w, r)
			} else {
				defaultUpstream.ServeHTTP(w, r)
			}
		}), nil
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
	w.Header().Add("Content-Type", "application/json")
	info := struct {
		Hostname    string               `json:"hostname,omitempty"`
		Request     *request             `json:"request,omitempty"`
		TLS         *tls.ConnectionState `json:"tls,omitempty"`
		JWTMetaData map[string][]*jwt    `json:"jwt_metadata,omitempty"`
	}{}
	info.Hostname, _ = os.Hostname()
	info.Request = newRequest(r)
	info.TLS = r.TLS
	info.JWTMetaData = make(map[string][]*jwt)
	for header, values := range r.Header {
		for _, value := range values {
			jwtMetadata := readJWT(value)
			if jwtMetadata == nil {
				continue
			}
			info.JWTMetaData[header] = append(info.JWTMetaData[header], jwtMetadata)
		}
	}
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
	Host       string      `json:"host"`
	URI        string      `json:"uri"`
	Protocol   string      `json:"protocol"`
	Header     http.Header `json:"header"`
	RemoteAddr string      `json:"remote_addr"`
	// TLS evtl.
}

func newRequest(r *http.Request) *request {
	return &request{
		Method:     r.Method,
		Host:       r.Host,
		URI:        r.RequestURI,
		Protocol:   r.Proto,
		Header:     r.Header,
		RemoteAddr: r.RemoteAddr,
	}
}

type jwt struct {
	Header   map[string]any `json:"header,omitempty"`
	Claims   map[string]any `json:"claims,omitempty"`
	Expiry   time.Time      `json:"expiry,omitempty"`
	IssuedAt time.Time      `json:"issued_at,omitempty"`
	Error    string         `json:"error,omitempty"`
}

func readJWT(tokenHeader string) *jwt {
	first, token, found := strings.Cut(tokenHeader, " ")
	if !found {
		// fallback if there is no auth scheme (e.g. Bearer)
		token = first
	}

	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return nil
	}

	jwt := &jwt{}

	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil
	}
	claims, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}

	err = json.Unmarshal(header, &jwt.Header)
	if err != nil {
		return nil
	}
	err = json.Unmarshal(claims, &jwt.Claims)
	if err != nil {
		jwt.Error = err.Error()
		return jwt
	}

	if exp, ok := jwt.Claims["exp"]; ok {
		expTime, _ := exp.(float64)
		jwt.Expiry = time.Unix(int64(expTime), 0)
	}
	if iat, ok := jwt.Claims["iat"]; ok {
		iatTime, _ := iat.(float64)
		jwt.IssuedAt = time.Unix(int64(iatTime), 0)
	}

	return jwt
}
