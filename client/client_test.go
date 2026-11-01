package client

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moisespsena-go/httpdx/internal"
	"github.com/moisespsena-go/httpdx/server"
)

// echoServer is a TCP server that writes back what it reads.
func echoServer(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()
	return l.Addr().String()
}

// idleKiller is a TCP proxy to target that closes a connection idle longer
// than idle — as a reverse proxy in front of the server does a websocket
// (nginx's proxy_read_timeout).
func idleKiller(t *testing.T, target string, idle time.Duration) string {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			a, err := l.Accept()
			if err != nil {
				return
			}
			b, err := net.Dial("tcp4", target)
			if err != nil {
				a.Close()
				continue
			}
			var (
				mu   sync.Mutex
				last = time.Now()
			)
			touch := func() { mu.Lock(); last = time.Now(); mu.Unlock() }
			pipe := func(dst, src net.Conn) {
				buf := make([]byte, 32*1024)
				for {
					n, err := src.Read(buf)
					if n > 0 {
						touch()
						if _, err := dst.Write(buf[:n]); err != nil {
							break
						}
					}
					if err != nil {
						break
					}
				}
				a.Close()
				b.Close()
			}
			go pipe(a, b)
			go pipe(b, a)
			go func() {
				for range time.Tick(idle / 4) {
					mu.Lock()
					dead := time.Since(last) > idle
					mu.Unlock()
					if dead {
						a.Close()
						b.Close()
						return
					}
				}
			}()
		}
	}()
	return l.Addr().String()
}

// bridge is a route of the client to the echo server, through the server and
// a proxy that closes what is idle for 300ms; keepAlive the client's.
func bridge(t *testing.T, keepAlive time.Duration) net.Conn {
	return bridgeWith(t, keepAlive, 0)
}

// bridgeWith is bridge with the server's keep alive too (0: none).
func bridgeWith(t *testing.T, keepAlive, serverKeepAlive time.Duration) net.Conn {
	t.Helper()
	mux := http.NewServeMux()
	h := server.New(map[string]*server.TCPSocketConfig{"echo": {Addr: echoServer(t)}},
		2*time.Second, 2*time.Second, 2*time.Second, false).WithKeepAlive(serverKeepAlive)
	mux.Handle(internal.ProxyPath, http.HandlerFunc(h.Proxy()))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	front := idleKiller(t, strings.TrimPrefix(srv.URL, "http://"), 300*time.Millisecond)
	u, _ := url.Parse("ws://" + front + internal.ProxyPath)
	done := make(chan *Listener, 1)
	lis := runService(done, 0, u, &RouteConfig{Name: "echo", LocalAddr: "127.0.0.1:0"}, keepAlive)
	if lis == nil {
		t.Fatal("no listener")
	}
	t.Cleanup(func() { lis.l.Close() })
	c, err := net.Dial("tcp4", lis.l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

// echo writes s and reads it back; an error when the session is gone.
func echo(c net.Conn, s string) (string, error) {
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := c.Write([]byte(s)); err != nil {
		return "", err
	}
	buf := make([]byte, len(s))
	_, err := io.ReadFull(c, buf)
	return string(buf), err
}

// A session idle longer than the proxy in front of the server allows stays
// open: the client pings it (keep alive).
func TestKeepAlive(t *testing.T) {
	c := bridge(t, 100*time.Millisecond)
	if got, err := echo(c, "a"); err != nil || got != "a" {
		t.Fatalf("first: %q %v", got, err)
	}
	time.Sleep(time.Second)
	if got, err := echo(c, "b"); err != nil || got != "b" {
		t.Fatalf("after idle: %q %v — the session dropped", got, err)
	}
}

// Without the keep alive the same idle session drops: what the bridge did.
func TestNoKeepAliveDrops(t *testing.T) {
	c := bridge(t, 0)
	if got, err := echo(c, "a"); err != nil || got != "a" {
		t.Fatalf("first: %q %v", got, err)
	}
	time.Sleep(time.Second)
	if got, err := echo(c, "b"); err == nil && got == "b" {
		t.Fatal("an idle session without keep alive survived: the test proxy does not close idle connections")
	}
}

// The server's keep alive alone keeps a client of before (without its own)
// alive behind the proxy.
func TestServerKeepAlive(t *testing.T) {
	c := bridgeWith(t, 0, 100*time.Millisecond)
	if got, err := echo(c, "a"); err != nil || got != "a" {
		t.Fatalf("first: %q %v", got, err)
	}
	time.Sleep(time.Second)
	if got, err := echo(c, "b"); err != nil || got != "b" {
		t.Fatalf("after idle: %q %v — the session dropped", got, err)
	}
}

// keep_alive of the config: empty the default, "0" none, a duration.
func TestKeepAliveOf(t *testing.T) {
	for in, want := range map[string]time.Duration{"": DefaultKeepAlive, "0": 0, "40s": 40 * time.Second} {
		if got, err := keepAliveOf(&Config{KeepAlive: in}); err != nil || got != want {
			t.Errorf("%q: %v %v", in, got, err)
		}
	}
	if _, err := keepAliveOf(&Config{KeepAlive: "x"}); err == nil {
		t.Error("a bad duration accepted")
	}
}
