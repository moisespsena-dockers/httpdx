package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/moisespsena-go/httpdx/internal"
)

// BufferSize for coybuffer and websocket
const BufferSize = 256 * 1024

// Handler handlers
type Handler struct {
	handlers          map[string]*TCPSocketConfig
	upgrader          websocket.Upgrader
	dialTimeout       time.Duration
	writeTimeout      time.Duration
	enableCompression bool
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
		handlers:     handlers,
		upgrader:     upgrader,
		dialTimeout:  dialTimeout,
		writeTimeout: writeTimeout,
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
			return
		}

		sck := h.handlers[name]
		if sck.Addr == "" {
			fail(fmt.Sprintf("%q is not registered"))
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
			http.Error(w, fmt.Sprintf("Could not connect upstream: %v", err), 500)
			return
		}

		rwc := &wsConnRW{c: wc}

		doneCh := make(chan bool)

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
		if mt != websocket.BinaryMessage {
			continue
		}
		w.r = r
		goto start
	}
}

func (w *wsConnRW) Write(p []byte) (n int, err error) {
	if err = w.c.WriteMessage(websocket.BinaryMessage, p); err == nil {
		return len(p), nil
	}
	return
}

func (w *wsConnRW) Close() error {
	return w.c.Close()
}
