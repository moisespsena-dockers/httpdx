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
	// KeepAlive is how often a connection pings the server while open
	// (a Go duration: "25s", "1m"): a reverse proxy in front of the server
	// (nginx: proxy_read_timeout, 60s by default) closes a websocket idle
	// longer than its timeout — an open session without traffic (psql, ssh)
	// dropped. Empty: DefaultKeepAlive; "0": never.
	KeepAlive string `yaml:"keep_alive"`
}
