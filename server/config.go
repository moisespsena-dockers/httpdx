package server

import "strconv"

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

type Config struct {
	Addr string `yaml:"addr"`
	// NotFound is a file path to handles unhandled requests.
	NotFound string `yaml:"not_found"`
	// NotFoundDisabled if value is true, disables handle unhandled requests.
	NotFoundDisabled bool `yaml:"not_found_disabled"`
	Routes           struct {
		Http       map[string]*HttpConfig      `yaml:"http"`
		TCPSockets map[string]*TCPSocketConfig `yaml:"tcp_sockets"`
	} `yaml:"routes"`
}
