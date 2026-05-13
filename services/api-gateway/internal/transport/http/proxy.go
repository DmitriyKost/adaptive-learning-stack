package httptransport

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"api-gateway/internal/domain"
)

type backendProxy struct {
	name    string
	target  *url.URL
	handler *httputil.ReverseProxy
}

func newBackendProxy(name, rawURL string, log *slog.Logger) (*backendProxy, error) {
	target, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if target.Scheme == "" || target.Host == "" {
		return nil, errors.New("backend url must include scheme and host")
	}

	director := func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = singleJoiningSlash(target.Path, req.URL.Path)
		if target.RawQuery == "" || req.URL.RawQuery == "" {
			req.URL.RawQuery = target.RawQuery + req.URL.RawQuery
		} else {
			req.URL.RawQuery = target.RawQuery + "&" + req.URL.RawQuery
		}
		req.Host = target.Host

		if prior, ok := req.Header["X-Forwarded-Host"]; ok {
			req.Header["X-Forwarded-Host"] = append(prior, req.Host)
		} else {
			req.Header.Set("X-Forwarded-Host", req.Host)
		}
		req.Header.Set("X-Gateway-Service", "api-gateway")
	}

	proxy := &httputil.ReverseProxy{
		Director: director,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   5 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Error("proxy request failed", "backend", name, "target", rawURL, "path", r.URL.Path, "error", err)
			writeError(w, domain.ErrBadGateway)
		},
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Set("X-Upstream-Service", name)
			return nil
		},
	}

	return &backendProxy{name: name, target: target, handler: proxy}, nil
}

func (p *backendProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.handler.ServeHTTP(w, r)
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	default:
		return a + b
	}
}
