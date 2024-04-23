package client

type AuthConfig struct {
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Disabled bool   `yaml:"disabled"`
}

type RouteConfig struct {
	Name      string      `yaml:"name"`
	LocalAddr string      `yaml:"local_addr"`
	Auth      *AuthConfig `yaml:"auth"`
	Disabled  bool        `yaml:"disabled"`
}

type Config struct {
	ServerURL string         `yaml:"server_url"`
	Routes    []*RouteConfig `yaml:"routes"`
	Auth      *AuthConfig    `yaml:"auth"`
}
