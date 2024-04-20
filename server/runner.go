package server

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/moisespsena-go/httpdx/internal"
)

func Serve(cfg *Config, args []string) (err error) {
	cfg.TCPSockets.Defaults()

	var (
		proxyHandler = New(
			cfg.TCPSockets.Routes,
			time.Second*time.Duration(cfg.TCPSockets.HandshakeTimeout),
			time.Second*time.Duration(cfg.TCPSockets.DialTimeout),
			time.Second*time.Duration(cfg.TCPSockets.WriteTimeout),
			cfg.TCPSockets.CompressionEnabled,
		)
		proxies     []string
		rootHandler http.Handler
	)

	http.Handle(internal.ProxyPath, http.HandlerFunc(proxyHandler.Proxy()))

	for pth, cfg := range cfg.HTTP.Routes {
		if cfg.Disabled {
			continue
		}

		var proxy http.Handler

		if cfg.Dir {
			pth = strings.TrimRight(pth, "/") + "/"
		}

		proxy, err = createReverseProxy(pth, cfg)
		if err != nil {
			return fmt.Errorf("create reverse proxy failed: %s", err)
		}

		if pth == "/" {
			rootHandler = proxy
		} else {
			http.Handle(pth, proxy)
		}

		proxies = append(proxies, fmt.Sprintf("HTTP %q 🡒 %s", pth, cfg.ToString(pth)))
	}

	for pth, sck := range cfg.TCPSockets.Routes {
		if sck.Disabled {
			continue
		}
		proxies = append(proxies, fmt.Sprintf("TCP %q 🡒 %s", pth, sck))
	}

	sort.Strings(proxies)

	if len(proxies) > 0 {
		log.Printf("Starting reverse proxy server on %s, with targets:\n  %s", cfg.Addr, strings.Join(proxies, "\n  "))
	} else {
		log.Printf("Starting reverse proxy server on port %s without targets", cfg.Addr)
	}

	if !cfg.NotFoundDisabled {
		fallback := func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, fallbackPage, r.URL.Path)
		}

		if cfg.NotFound != "" {
			fallback = func(w http.ResponseWriter, r *http.Request) {
				http.ServeFile(w, r, cfg.NotFound)
			}
		}

		var (
			handlers       Handlers
			hasRootHandler = rootHandler != nil
		)

		if hasRootHandler {
			handlers = append(handlers, rootHandler)
		}

		handlers = append(handlers, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Httpdx-Handle-Fallback") != "false" {
				fallback(w, r)
			}
		}))

		rootHandler = handlers
	}

	if rootHandler != nil {
		http.Handle("/", rootHandler)
	}

	return http.ListenAndServe(cfg.Addr, nil)
}

type Handlers []http.Handler

func (h Handlers) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f := reflect.ValueOf(w).Elem().FieldByName("wroteHeader")
	for _, h := range h {
		if f.Bool() {
			return
		}
		h.ServeHTTP(w, r)
	}
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

const fallbackPage = `<!DOCTYPE html>
<html>
<head>
<title>Welcome to HTTPDx!</title>
<style>
    body {
        width: 35em;
        margin: 0 auto;
        font-family: Tahoma, Verdana, Arial, sans-serif;
    }

	code {
		background-color: #f7dfdf;
		padding: 3px;
		border: 1px solid #f9b9b9;
	}
</style>
</head>
<body>
<h1>Welcome to HTTPDx!</h1>
<p>If you see this page, the HTTPDx web server is successfully installed and
working. Further configuration is required.</p>

<p>For online documentation and support please refer to
<a href="https://github.com/moisespsena-go/httpdx">HTTPDx</a>.<br/>

<p style="color:red">Warning: The requested page <code>%s</code> is unhandled.</strong></p>

<p><em>Thank you for using HTTPDx.</em></p>
</body>
</html>`
