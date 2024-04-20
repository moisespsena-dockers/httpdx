package server

import (
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
		name := r.URL.Query().Get("name")
		if name == "" {
			http.Error(w, "name is blank", http.StatusPreconditionFailed)
		}

		if name == internal.TestService {
			conn, err := h.upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			conn.PingHandler()("!!test!!")
			return
		}

		sck := h.handlers[name]
		if sck.Addr == "" {
			http.Error(w, fmt.Sprintf("%q is not registered"), http.StatusPreconditionFailed)
		}

		s, err := net.DialTimeout("tcp", sck.Addr, h.dialTimeout)

		if err != nil {
			http.Error(w, fmt.Sprintf("Could not connect upstream: %v", err), 500)
			return
		}

		conn, err := h.upgrader.Upgrade(w, r, nil)
		if err != nil {
			s.Close()
			return
		}

		rwc := &wsConnRW{c: conn}

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
		conn.Close()
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
