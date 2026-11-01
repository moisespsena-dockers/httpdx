// Command front is a TCP proxy to target that closes a connection idle longer
// than -idle — as a reverse proxy in front of the httpdx server does a
// websocket (nginx's proxy_read_timeout): the front of the compose tests
// (test/compose).
package main

import (
	"flag"
	"log"
	"net"
	"sync"
	"time"
)

func main() {
	addr := flag.String("listen", ":8080", "address to listen on")
	target := flag.String("target", "", "address to proxy to")
	idle := flag.Duration("idle", 3*time.Second, "close a connection idle longer than this")
	flag.Parse()
	l, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("front on %s to %s, idle %v", *addr, *target, *idle)
	for {
		a, err := l.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go serve(a, *target, *idle)
	}
}

func serve(a net.Conn, target string, idle time.Duration) {
	b, err := net.Dial("tcp", target)
	if err != nil {
		log.Printf("dial %s: %v", target, err)
		a.Close()
		return
	}
	var (
		mu   sync.Mutex
		last = time.Now()
		once sync.Once
	)
	closeAll := func(why string) {
		once.Do(func() {
			log.Printf("%s: %s", a.RemoteAddr(), why)
			a.Close()
			b.Close()
		})
	}
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
		closeAll("closed")
	}
	go pipe(a, b)
	go pipe(b, a)
	t := time.NewTicker(idle / 4)
	defer t.Stop()
	for range t.C {
		mu.Lock()
		dead := time.Since(last) > idle
		mu.Unlock()
		if dead {
			closeAll("idle: dropped")
			return
		}
	}
}
