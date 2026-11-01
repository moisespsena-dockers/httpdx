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

// The handlers of a request: the first that answers ends it — whatever the
// writer (reading net/http's private field, it panicked on any other).
func TestHandlers(t *testing.T) {
	var second bool
	h := Handlers{
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/a" {
				w.WriteHeader(http.StatusTeapot)
			}
		}),
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			second = true
			w.WriteHeader(http.StatusNotFound)
		}),
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/a", nil))
	if rec.Code != http.StatusTeapot || second {
		t.Errorf("the first answered: %d, the second ran %v", rec.Code, second)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/b", nil))
	if rec.Code != http.StatusNotFound || !second {
		t.Errorf("the first did not answer: %d, the second ran %v", rec.Code, second)
	}
}

// keep_alive: empty the default, "0" none, a duration.
func TestKeepAliveOf(t *testing.T) {
	for in, want := range map[string]time.Duration{"": DefaultKeepAlive, "0": 0, "40s": 40 * time.Second} {
		if got, err := KeepAliveOf(in); err != nil || got != want {
			t.Errorf("%q: %v %v", in, got, err)
		}
	}
	if _, err := KeepAliveOf("x"); err == nil {
		t.Error("a bad duration accepted")
	}
}
