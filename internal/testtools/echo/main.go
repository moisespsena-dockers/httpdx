// Command echo is a TCP server that writes back what it reads: the upstream
// of the compose tests (test/compose), in the place of a database or ssh.
package main

import (
	"flag"
	"io"
	"log"
	"net"
)

func main() {
	addr := flag.String("listen", ":7000", "address to listen on")
	flag.Parse()
	l, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("echo on %s", *addr)
	for {
		c, err := l.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go func() { defer c.Close(); io.Copy(c, c) }()
	}
}
