package server

import (
	"fmt"
	"strconv"
)

type HttpConfig struct {
	Addr       string `yaml:"addr"`
	PathStrip  bool   `yaml:"path_strip"`
	PathHeader string `yaml:"path_header"`
	Dir        string `yaml:"dir"`
	// PathOverride overrides real file path.
	// This formatter haves 3 values:
	// 1. The requested path
	// 2. The DIR
	// 3. The route PATH
	PathOverride string `yaml:"path_override"`
	Disabled     bool   `yaml:"disabled"`
}

func (c *HttpConfig) ToString(dir string) string {
	if c.Dir != "" {
		return fmt.Sprintf("STATIC %s", c.Dir)
	}

	s := c.Addr
	if c.PathStrip {
		s += " [" + strconv.Quote(dir) + "]"
	}
	return s
}

type TCPSocketConfig struct {
	Addr     string      `yaml:"addr"`
	Auth     *AuthConfig `yaml:"auth"`
	Disabled bool        `yaml:"disabled"`
}

func (c *TCPSocketConfig) String() string {
	return c.Addr
}

type AuthConfig struct {
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Disabled bool   `yaml:"disabled"`
}

type TCPSocketsConfig struct {
	HandshakeTimeout   uint8                       `yaml:"handshake_timeout"`
	DialTimeout        uint8                       `yaml:"dial_timeout"`
	WriteTimeout       uint8                       `yaml:"write_timeout"`
	CompressionEnabled bool                        `yaml:"compression_enabled"`
	Auth               *AuthConfig                 `yaml:"auth"`
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
