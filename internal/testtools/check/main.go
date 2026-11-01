// Command check runs the compose tests (test/compose): sessions through the
// httpdx clients idle longer than the front allows — kept alive by the client,
// by the server, and, with neither, dropped (the control) —, a route with
// authentication, and the server's answers to a route not registered, to an
// upstream down and to a wrong password.
package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

var failed bool

func fail(format string, a ...any) {
	failed = true
	log.Printf("FAIL: "+format, a...)
}

func ok(format string, a ...any) { log.Printf("ok: "+format, a...) }

// echo writes s and reads it back.
func echo(c net.Conn, s string) error {
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write([]byte(s)); err != nil {
		return err
	}
	buf := make([]byte, len(s))
	if _, err := io.ReadFull(c, buf); err != nil {
		return err
	}
	if string(buf) != s {
		return fmt.Errorf("%q back, not %q", buf, s)
	}
	return nil
}

// dial waits for addr to accept.
func dial(addr string) (net.Conn, error) {
	var err error
	for i := 0; i < 50; i++ {
		var c net.Conn
		if c, err = net.Dial("tcp", addr); err == nil {
			return c, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil, err
}

// session opens a session through the client at addr, idles idle, and says
// whether it is still there.
func session(name, addr string, idle time.Duration) (alive bool) {
	c, err := dial(addr)
	if err != nil {
		fail("%s: dial %s: %v", name, addr, err)
		return false
	}
	defer c.Close()
	if err := echo(c, "before"); err != nil {
		fail("%s: first echo: %v", name, err)
		return false
	}
	time.Sleep(idle)
	return echo(c, "after-idle") == nil
}

// serverMessage is the first message the server answers route with.
func serverMessage(server, route, user, password string) string {
	h := http.Header{}
	if user != "" {
		h.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(user+":"+password)))
	}
	var (
		c   *websocket.Conn
		err error
	)
	for i := 0; i < 50; i++ {
		if c, _, err = websocket.DefaultDialer.Dial("ws://"+server+"/!?name="+route, h); err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err != nil {
		return "dial: " + err.Error()
	}
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, msg, err := c.ReadMessage()
	if err != nil {
		return "read: " + err.Error()
	}
	return string(msg)
}

func main() {
	idle := flag.Duration("idle", 7*time.Second, "how long a session idles (longer than the front's)")
	flag.Parse()

	// the sessions, all at once
	type scenario struct {
		name, addr string
		alive      bool
	}
	scenarios := []scenario{
		{"client keep alive, server none", "client-keep:25000", true},
		{"client none, server keep alive", "client-old:25000", true},
		{"client and server keep alive, with auth", "client-auth:25000", true},
		{"neither (control)", "client-control:25000", false},
	}
	results := make(chan string, len(scenarios))
	for _, s := range scenarios {
		go func(s scenario) {
			got := session(s.name, s.addr, *idle)
			switch {
			case got == s.alive && got:
				ok("%s: alive after %v idle", s.name, *idle)
			case got == s.alive:
				ok("%s: dropped after %v idle, as expected", s.name, *idle)
			case got:
				fail("%s: alive after %v idle — the front does not drop idle connections?", s.name, *idle)
			default:
				fail("%s: dropped after %v idle", s.name, *idle)
			}
			results <- s.name
		}(s)
	}

	// the server's answers
	for _, c := range []struct{ name, route, user, password, want string }{
		{"a route not registered", "nope", "", "", `ERROR: "nope" is not registered`},
		{"an upstream down", "down", "", "", "ERROR: could not connect upstream"},
		{"a wrong password", "secured", "tester", "wrong", "ERROR: invalid username or password"},
	} {
		if got := serverMessage("server-keep:80", c.route, c.user, c.password); strings.HasPrefix(got, c.want) {
			ok("%s: %q", c.name, got)
		} else {
			fail("%s: %q, want %q", c.name, got, c.want)
		}
	}

	for range scenarios {
		<-results
	}
	if failed {
		log.Print("FAILED")
		os.Exit(1)
	}
	log.Print("PASSED")
}
