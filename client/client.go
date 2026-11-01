package client

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/moisespsena-dockers/httpdx/internal"
)

const pingPayload = "!!test!!"

// DefaultKeepAlive is how often a connection pings the server when the config
// says nothing (Config.KeepAlive): under the 60s a reverse proxy commonly
// closes an idle websocket after.
const DefaultKeepAlive = 25 * time.Second

// writeWait is how long a ping may take to be written.
const writeWait = 10 * time.Second

type Listener struct {
	id string
	l  net.Listener
}

// keepAliveOf is the interval of cfg.KeepAlive: DefaultKeepAlive when empty, 0
// (none) when "0".
func keepAliveOf(cfg *Config) (time.Duration, error) {
	s := strings.TrimSpace(cfg.KeepAlive)
	if s == "" {
		return DefaultKeepAlive, nil
	}
	if s == "0" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("keep_alive: %w", err)
	}
	return d, nil
}

func Run(cfg *Config) (err error) {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	serverURL := cfg.ServerURL + internal.ProxyPath

	keepAlive, err := keepAliveOf(cfg)
	if err != nil {
		return err
	}

	var u *url.URL
	if u, err = url.Parse(serverURL); err != nil {
		return
	}

	log.Println("Server URL: " + u.String())

	if strings.HasPrefix(u.Scheme, "http") {
		u.Scheme = "ws" + u.Scheme[4:]
	}

	log.Println("Connection URL: " + u.String())
	log.Printf("Keep alive: %v", keepAlive)
	u.RawQuery = "name=" + internal.TestRoute
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("test_service dial: %s", err)
	}

	c.SetPongHandler(func(appData string) error {
		if appData != pingPayload {
			return fmt.Errorf("test_service: expected ping message payload")
		}
		return nil
	})
	if err = c.WriteMessage(websocket.PingMessage, []byte(pingPayload)); err != nil {
		return fmt.Errorf("test_service: read message: %s", err)
	}
	c.Close()

	var (
		// the listeners serving, by themselves: a route disabled, or that
		// could not listen, has none — the ones that do are not numbered
		// by the routes
		listeners = map[*Listener]bool{}
		done      = make(chan *Listener)
	)

	for i, route := range cfg.Routes {
		if route.Disabled {
			continue
		}

		if route.Auth == nil {
			route.Auth = cfg.Auth
		}

		if l := runService(done, i, u, route, keepAlive); l != nil {
			listeners[l] = true
		}
	}

	for len(listeners) > 0 {
		select {
		case <-interrupt:
			for l := range listeners {
				l.l.Close()
			}
		case l := <-done:
			delete(listeners, l)
		}
	}

	return
}

func runService(done chan<- *Listener, i int, u *url.URL, cfg *RouteConfig, keepAlive time.Duration) *Listener {
	id := "route #" + strconv.Itoa(i) + " {" + cfg.Name + " 🡒 " + cfg.LocalAddr + "}:"
	log.Println(id, "started")

	l, err := net.Listen("tcp4", cfg.LocalAddr)
	if err != nil {
		log.Printf("%s: listen: %v", id, err)
		return nil
	}

	lis := &Listener{id, l}

	go func() {
		defer func() {
			l.Close()
			log.Println(id, "done")
			done <- lis
		}()
		for {
			c, err := l.Accept()
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					// a temporary failure (too many open files…): not the
					// end of the route
					log.Printf("%s: accept: %v", id, err)
					time.Sleep(100 * time.Millisecond)
					continue
				}
				if !strings.HasSuffix(err.Error(), "use of closed network connection") {
					log.Printf("%s: accept: %v", id, err)
				}
				return
			}
			{
				u := *u
				u.RawQuery = "name=" + cfg.Name
				go handleConnection(u, id, c, cfg.Auth, keepAlive)
			}
		}
	}()

	return lis
}

func handleConnection(u url.URL, id string, con net.Conn, auth *AuthConfig, keepAlive time.Duration) {
	log.Printf("%s: serving %s", id, con.RemoteAddr().String())

	var header http.Header

	if auth != nil && !auth.Disabled {
		header = map[string][]string{}
		header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(auth.User+":"+auth.Password)))
	}

	c, _, err := websocket.DefaultDialer.Dial(u.String(), header)
	if err != nil {
		log.Printf(id+": dial: %v", err)
		con.Close()
		return
	}

	// closing one side closes the other: once
	var closeOnce sync.Once
	closeAll := func() {
		closeOnce.Do(func() {
			c.Close()
			con.Close()
		})
	}
	defer closeAll()

	stop := make(chan struct{})
	defer close(stop)

	if keepAlive > 0 {
		// the server's pong (gorilla answers a ping by itself) — or any
		// message — says the connection is alive; none for 3 intervals: a
		// connection dead in silence, closed, so that the local client
		// knows instead of waiting
		alive := func() { _ = c.SetReadDeadline(time.Now().Add(3 * keepAlive)) }
		alive()
		c.SetPongHandler(func(string) error { alive(); return nil })

		go func() {
			ticker := time.NewTicker(keepAlive)
			defer ticker.Stop()
			for {
				select {
				case <-stop:
					return
				case <-ticker.C:
					// WriteControl may be called while io.Copy writes
					if err := c.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait)); err != nil {
						log.Printf("%s: keep alive: %v", id, err)
						closeAll()
						return
					}
				}
			}
		}()

		go readLoop(id, c, con, closeAll, alive)
	} else {
		go readLoop(id, c, con, closeAll, func() {})
	}

	defer func() {
		log.Printf("%s: %s: done", id, con.RemoteAddr().String())
	}()
	io.Copy(&wsw{c: c}, con)
}

// readLoop writes to the local connection what the server sends — its text
// messages are its log, logged —, until either side fails: then both are
// closed.
func readLoop(id string, c *websocket.Conn, con net.Conn, closeAll func(), alive func()) {
	defer closeAll()
	for {
		t, msg, err := c.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure) &&
				!strings.HasSuffix(err.Error(), "use of closed network connection") {
				log.Printf(id+": read_message: %v", err)
			}
			return
		}
		alive()
		switch t {
		case websocket.TextMessage:
			log.Print(id + ": REMOTE MESSAGE: " + string(msg))
		case websocket.BinaryMessage:
			if _, err := con.Write(msg); err != nil {
				if !strings.HasSuffix(err.Error(), "use of closed network connection") {
					log.Printf(id+": write_message: %v", err)
				}
				return
			}
		}
	}
}

type wsw struct {
	c *websocket.Conn
}

func (w *wsw) Write(p []byte) (_ int, err error) {
	err = w.c.WriteMessage(websocket.BinaryMessage, p)
	return len(p), err
}
