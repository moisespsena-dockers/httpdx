package server

import (
	"strconv"
)

type HttpConfig struct {
	Addr       string `yaml:"addr"`
	Dir        bool   `yaml:"dir"`
	PathHeader string `yaml:"path_header"`
	Disabled   bool   `yaml:"disabled"`
}

func (c *HttpConfig) ToString(dir string) string {
	s := c.Addr
	if c.Dir {
		s += " [" + strconv.Quote(dir) + "]"
	}
	return s
}

type TCPSocketConfig struct {
	Addr     string `yaml:"addr"`
	Disabled bool   `yaml:"disabled"`
}

func (c *TCPSocketConfig) String() string {
	return c.Addr
}

type TCPSocketsConfig struct {
	HandshakeTimeout   uint8                       `yaml:"handshake_timeout"`
	DialTimeout        uint8                       `yaml:"dial_timeout"`
	WriteTimeout       uint8                       `yaml:"write_timeout"`
	CompressionEnabled bool                        `yaml:"compression_enabled"`
	Routes             map[string]*TCPSocketConfig `yaml:"routes"`
}

func (c *TCPSocketsConfig) Defaults() {
	if c.HandshakeTimeout == 0 {
		c.HandshakeTimeout = 5
	}
	if c.DialTimeout == 0 {
		c.DialTimeout = 5
	}
	if c.WriteTimeout == 0 {
		c.WriteTimeout = 5
	}
}

type Config struct {
	Addr string `yaml:"addr"`
	// NotFound is a file path to handles unhandled requests.
	NotFound string `yaml:"not_found"`
	// NotFoundDisabled if value is true, disables handle unhandled requests.
	NotFoundDisabled bool             `yaml:"not_found_disabled"`
	TCPSockets       TCPSocketsConfig `yaml:"tcp_sockets"`
	HTTP             struct {
		Routes map[string]*HttpConfig `yaml:"routes"`
	} `yaml:"http"`
}
