package server

import "strconv"

type HttpConfig struct {
	Addr       string `yaml:"addr"`
	Dir        bool   `yaml:"dir"`
	PathHeader string `yaml:"path_header"`
}

func (c *HttpConfig) ToString(dir string) string {
	s := c.Addr
	if c.Dir {
		s += " [" + strconv.Quote(dir) + "]"
	}
	return s
}

type Config struct {
	Addr   string `yaml:"addr"`
	Routes struct {
		Http       map[string]HttpConfig `yaml:"http"`
		TCPSockets map[string]string     `yaml:"tcp_sockets"`
	} `yaml:"routes"`
}
