package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/moisespsena-go/httpdx/internal"
)

// message is the first message the proxy answers name with.
func message(t *testing.T, handlers map[string]*TCPSocketConfig, name string) string {
	t.Helper()
	h := New(handlers, time.Second, 200*time.Millisecond, time.Second, false)
	srv := httptest.NewServer(http.HandlerFunc(h.Proxy()))
	t.Cleanup(srv.Close)
	c, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+internal.ProxyPath+"?name="+name, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return string(msg)
}

// A route not registered is told so (it was a panic); one whose upstream does
// not answer, too — by a message of the websocket, not an HTTP answer it no
// longer reaches.
func TestProxyErrors(t *testing.T) {
	if got := message(t, nil, "nope"); got != `ERROR: "nope" is not registered` {
		t.Errorf("not registered: %q", got)
	}
	got := message(t, map[string]*TCPSocketConfig{"down": {Addr: "127.0.0.1:1"}}, "down")
	if !strings.HasPrefix(got, "ERROR: could not connect upstream") {
		t.Errorf("upstream down: %q", got)
	}
}
