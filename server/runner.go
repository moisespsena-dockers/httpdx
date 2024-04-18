package server

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/moisespsena-go/httpdx/internal"
)

func Serve(cfg *Config) (err error) {
	proxyHandler := New(
		cfg.Routes.TCPSockets,
		5*time.Second,
		5*time.Second,
		5*time.Second,
		false,
	)

	http.Handle(internal.ProxyPath, http.HandlerFunc(proxyHandler.Proxy()))

	var (
		proxies []string
	)

	for pth, cfg := range cfg.Routes.Http {
		var proxy http.Handler

		if cfg.Dir {
			pth = strings.TrimRight(pth, "/") + "/"
		}

		proxy, err = createReverseProxy(pth, &cfg)
		if err != nil {
			return fmt.Errorf("create reverse proxy failed: %s", err)
		}
		http.Handle(pth, proxy)

		proxies = append(proxies, fmt.Sprintf("HTTP %q 🡒 %s", pth, cfg.ToString(pth)))
	}

	for pth, addr := range cfg.Routes.TCPSockets {
		proxies = append(proxies, fmt.Sprintf("TCP %q 🡒 %q", pth, addr))
	}

	sort.Strings(proxies)

	if len(proxies) > 0 {
		log.Printf("Starting reverse proxy server on %s, with targets:\n  %s", cfg.Addr, strings.Join(proxies, "\n  "))
	} else {
		log.Printf("Starting reverse proxy server on port %s without targets", cfg.Addr)
	}

	return http.ListenAndServe(cfg.Addr, nil)
}

func createReverseProxy(pth string, cfg *HttpConfig) (http.Handler, error) {
	targetURL, err := url.Parse("http://" + cfg.Addr)
	if err != nil {
		return nil, err
	}
	rv := httputil.NewSingleHostReverseProxy(targetURL)
	if cfg.Dir {
		headerName := cfg.PathHeader
		if headerName == "" {
			headerName = "X-Forwarded-Prefix"
		}

		pth2 := strings.TrimRight(pth, "/")

		oldDirector := rv.Director
		rv.Director = func(r *http.Request) {
			oldDirector(r)
			if r.URL.Path == pth2 {
				r.URL.Path = "/"
			} else {
				r.URL.Path = strings.TrimPrefix(r.URL.Path, pth2)
			}
			if s := r.Header.Get(headerName); s != "" {
				r.Header.Set(headerName, path.Join(path.Clean(s), pth2))
			} else {
				r.Header.Set(headerName, pth2)
			}
		}
	}
	rv.ErrorHandler = func(writer http.ResponseWriter, request *http.Request, err error) {
		if err.Error() != "EOF" {
			http.Error(writer, err.Error(), http.StatusBadGateway)
		}
	}
	return rv, nil
}
