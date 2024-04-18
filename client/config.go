package client

type ServiceConfig struct {
	Name      string `yaml:"name" yaml:"name"`
	LocalAddr string `yaml:"local_addr" yaml:"local_addr"`
}

type Config struct {
	ServerURL string          `yaml:"server_url"`
	Services  []ServiceConfig `yaml:"services"`
}
