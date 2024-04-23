package client

import (
	"encoding/base64"
	"errors"
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

	"github.com/gorilla/websocket"
	"github.com/moisespsena-go/httpdx/internal"
)

const pingPayload = "!!test!!"

type Listener struct {
	id string
	l  net.Listener
}

func Run(cfg *Config) (err error) {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	serverURL := cfg.ServerURL + internal.ProxyPath

	var u *url.URL
	if u, err = url.Parse(serverURL); err != nil {
		return
	}

	log.Println("Server URL: " + u.String())

	if strings.HasPrefix(u.Scheme, "http") {
		u.Scheme = "ws" + u.Scheme[4:]
	}

	log.Println("Connection URL: " + u.String())
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
		listeners []*Listener
		done      = make(chan int)
		doneCount int
	)

	for i, route := range cfg.Routes {
		if route.Disabled {
			continue
		}

		if route.Auth == nil {
			route.Auth = cfg.Auth
		}

		if l := runService(func() {
			done <- i + 1
		}, i, u, route); l != nil {
			listeners = append(listeners, l)
		}
	}

	for doneCount < len(listeners) {
		select {
		case <-interrupt:
			for _, l := range listeners {
				if l != nil {
					l.l.Close()
				}
			}
		case i := <-done:
			listeners[i-1] = nil
			doneCount++
		}
	}

	return
}

func runService(done func(), i int, u *url.URL, cfg *RouteConfig) (_ *Listener) {
	id := "route #" + strconv.Itoa(i) + " {" + cfg.Name + " 🡒 " + cfg.LocalAddr + "}:"
	log.Println(id, "started")

	l, err := net.Listen("tcp4", cfg.LocalAddr)
	if err != nil {
		log.Printf("%s: listen: %v", id, err)
		return
	}

	go func() {
		defer func() {
			l.Close()
			log.Println(id, "done")
			done()
		}()
		for {
			c, err := l.Accept()
			if err != nil {
				if !strings.HasSuffix(err.Error(), "use of closed network connection") {
					log.Printf("%s: accept: %v", id, err)
				}
				return
			}
			{
				u := *u
				u.RawQuery = "name=" + cfg.Name
				go handleConnection(u, id, c, cfg.Auth)
			}
		}
	}()

	return &Listener{id, l}
}

func handleConnection(u url.URL, id string, con net.Conn, auth *AuthConfig) {
	log.Printf("%s: serving %s", id, con.RemoteAddr().String())

	var header http.Header

	if auth != nil {
		header = map[string][]string{}
		header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(auth.User+":"+auth.Password)))
	}

	c, _, err := websocket.DefaultDialer.Dial(u.String(), header)
	if err != nil {
		log.Printf(id+": dial: %v", err)
		return
	}

	defer c.Close()

	go func() {
		var read = func() (msg []byte, err error) {
			defer func() {
				if r := recover(); r != nil {
					switch t := r.(type) {
					case string:
						if t == "repeated read on failed websocket connection" {
							err = nil
						} else {
							err = errors.New(t)
						}
					case error:
						err = t
					default:
						err = fmt.Errorf("%v", t)
					}
				}
			}()
			for t := websocket.TextMessage; err == nil && t != websocket.BinaryMessage; {
				t, msg, err = c.ReadMessage()
				if t == websocket.TextMessage {
					// is a log message
					log.Print(id + ": REMOTE MESSAGE: " + string(msg))
				}
			}
			return
		}
		for {
			message, err := read()
			if err != nil {
				if !strings.HasSuffix(err.Error(), "use of closed network connection") {
					log.Printf(id+": read_message: %v", err)
				}
				con.Close()
				return
			} else {
				if _, err := con.Write(message); err != nil {
					log.Printf(id+": write_message: %v", err)
					return
				}
			}
		}
	}()

	defer func() {
		log.Printf("%s: %s: done", id, con.RemoteAddr().String())
	}()
	io.Copy(&wsw{c: c}, con)
}

type wsw struct {
	c *websocket.Conn
}

func (w *wsw) Write(p []byte) (_ int, err error) {
	err = w.c.WriteMessage(websocket.BinaryMessage, p)
	return len(p), err
}
