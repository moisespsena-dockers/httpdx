package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/moisespsena-dockers/httpdx/internal"
)

// BufferSize for coybuffer and websocket
const BufferSize = 256 * 1024

// DefaultKeepAlive is how often the server pings a connection when the config
// says nothing (TCPSocketsConfig.KeepAlive).
const DefaultKeepAlive = 25 * time.Second

// Handler handlers
type Handler struct {
	handlers          map[string]*TCPSocketConfig
	upgrader          websocket.Upgrader
	dialTimeout       time.Duration
	writeTimeout      time.Duration
	enableCompression bool
	// keepAlive is how often a connection is pinged; 0, never.
	keepAlive time.Duration
}

// KeepAliveOf is the interval of s: DefaultKeepAlive when empty, 0 (none) when
// "0".
func KeepAliveOf(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	switch s {
	case "":
		return DefaultKeepAlive, nil
	case "0":
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("keep_alive: %w", err)
	}
	return d, nil
}

// WithKeepAlive sets how often a connection is pinged (0: never).
func (h *Handler) WithKeepAlive(d time.Duration) *Handler {
	h.keepAlive = d
	return h
}

// New new handler
func New(
	handlers map[string]*TCPSocketConfig,
	handshakeTimeout time.Duration,
	dialTimeout time.Duration,
	writeTimeout time.Duration,
	enableCompression bool) *Handler {

	upgrader := websocket.Upgrader{
		EnableCompression: enableCompression,
		ReadBufferSize:    BufferSize,
		WriteBufferSize:   BufferSize,
		HandshakeTimeout:  handshakeTimeout,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	return &Handler{
		handlers:          handlers,
		upgrader:          upgrader,
		dialTimeout:       dialTimeout,
		writeTimeout:      writeTimeout,
		enableCompression: enableCompression,
		keepAlive:         DefaultKeepAlive,
	}
}

// Proxy proxy handler
func (h *Handler) Proxy() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		wc, err := h.upgrader.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, "WEBSOCKET failed: "+err.Error(), http.StatusPreconditionFailed)
			return
		}

		fail := func(msg string) {
			wc.WriteMessage(websocket.TextMessage, []byte("ERROR: "+msg))
			wc.Close()
		}

		name := r.URL.Query().Get("name")
		if name == "" {
			fail("name is blank")
			return
		}

		if name == internal.TestRoute {
			wc.PingHandler()("!!test!!")
			wc.Close()
			return
		}

		// a name not registered — nil — was a panic
		sck := h.handlers[name]
		if sck == nil || sck.Addr == "" {
			fail(fmt.Sprintf("%q is not registered", name))
			return
		}

		if sck.Auth != nil && !sck.Auth.Disabled {
			username, password, _ := r.BasicAuth()
			// Calculate SHA-256 hashes for the provided and expected
			// usernames and passwords.
			usernameHash := sha256.Sum256([]byte(username))
			passwordHash := sha256.Sum256([]byte(password))
			expectedUsernameHash := sha256.Sum256([]byte(sck.Auth.User))
			expectedPasswordHash := sha256.Sum256([]byte(sck.Auth.Password))

			usernameMatch := (subtle.ConstantTimeCompare(usernameHash[:], expectedUsernameHash[:]) == 1)
			passwordMatch := (subtle.ConstantTimeCompare(passwordHash[:], expectedPasswordHash[:]) == 1)
			if !usernameMatch || !passwordMatch {
				fail("invalid username or password")
				return
			}
		}

		s, err := net.DialTimeout("tcp", sck.Addr, h.dialTimeout)

		if err != nil {
			// the connection is a websocket now: an HTTP answer no longer
			// reaches the client
			fail(fmt.Sprintf("could not connect upstream: %v", err))
			return
		}

		rwc := &wsConnRW{c: wc, writeTimeout: h.writeTimeout}

		doneCh := make(chan bool)
		stop := make(chan struct{})
		defer close(stop)

		if h.keepAlive > 0 {
			// the client's pong (or any message) says it is alive; none for 3
			// intervals: gone without closing — the connection closed
			alive := func() { _ = wc.SetReadDeadline(time.Now().Add(3 * h.keepAlive)) }
			alive()
			rwc.alive = alive
			wc.SetPongHandler(func(string) error { alive(); return nil })
			go func() {
				t := time.NewTicker(h.keepAlive)
				defer t.Stop()
				for {
					select {
					case <-stop:
						return
					case <-t.C:
						// WriteControl may be called while io.Copy writes
						if err := wc.WriteControl(websocket.PingMessage, nil, time.Now().Add(h.writeTimeout)); err != nil {
							wc.Close()
							s.Close()
							return
						}
					}
				}
			}()
		}

		// websocket -> server
		go func() {
			defer func() { doneCh <- true }()
			io.Copy(s, rwc)
		}()

		// server -> websocket
		go func() {
			defer func() { doneCh <- true }()
			io.Copy(rwc, s)
		}()

		<-doneCh
		s.Close()
		wc.Close()
		<-doneCh
	}

}

type wsConnRW struct {
	c *websocket.Conn
	r io.Reader
	// writeTimeout bounds a write (0: none): a client that stopped reading
	// does not hold the connection forever
	writeTimeout time.Duration
	// alive is told of every message read (nil: none)
	alive func()
}

func (w *wsConnRW) Read(p []byte) (n int, err error) {
start:
	if w.r != nil {
		if n, err = w.r.Read(p); err == io.EOF {
			w.r = nil
		} else {
			return
		}
	}

	var (
		mt int
		r  io.Reader
	)

	for {
		mt, r, err = w.c.NextReader()
		if websocket.IsCloseError(err,
			websocket.CloseNormalClosure,   // Normal.
			websocket.CloseAbnormalClosure, // OpenSSH killed proxy client.
		) {
			return 0, io.EOF
		}
		if err != nil {
			return
		}
		if w.alive != nil {
			w.alive()
		}
		if mt != websocket.BinaryMessage {
			continue
		}
		w.r = r
		goto start
	}
}

func (w *wsConnRW) Write(p []byte) (n int, err error) {
	if w.writeTimeout > 0 {
		_ = w.c.SetWriteDeadline(time.Now().Add(w.writeTimeout))
	}
	if err = w.c.WriteMessage(websocket.BinaryMessage, p); err == nil {
		return len(p), nil
	}
	return
}

func (w *wsConnRW) Close() error {
	return w.c.Close()
}
