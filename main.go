package main

import (
	"errors"
	"flag"
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/moisespsena-go/httpdx/client"
	"github.com/moisespsena-go/httpdx/server"
	"gopkg.in/yaml.v3"
)

var configFile = filepath.Base(os.Args[0]) + ".yml"

func main() {
	fs := flag.NewFlagSet(filepath.Base(os.Args[0]), flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage:\n")
		fmt.Fprintf(fs.Output(), "%s COMMAND [OPTIONS] ARG...\n\n", fs.Name())
		fmt.Fprintf(fs.Output(), "Available commands:\n"+
			"  server:        run as server.\n"+
			"  client:        run as client.\n"+
			"  create-config: create config file.\n\n")
		fmt.Fprintf(fs.Output(), "Default Options:\n")
		fs.PrintDefaults()
	}
	fs.StringVar(&configFile, "config", configFile, "YAML configuration file")

	if err := fs.Parse(os.Args[1:]); err != nil {
		if err.Error() == "flag: help requested" {
			return
		}
	}

	var (
		args       = fs.Args()
		cfg        Config
		readConfig = func() {
			data, err := os.ReadFile(configFile)

			if err != nil {
				log.Fatal(err)
			}

			if err = yaml.Unmarshal(data, &cfg); err != nil {
				log.Fatal(err)
			}
		}
		err error
	)

	if len(args) > 0 {
		switch args[0] {
		case "server":
			readConfig()
			if err = runServer(fs, &cfg.Server, args[1:]); err != nil {
				log.Fatal(err)
			}
		case "client":
			readConfig()
			if err = runClient(fs, &cfg.Client, args[1:]); err != nil {
				log.Fatal(err)
			}
		case "create-config":
			if err = runCreateConfig(fs, args[1:]); err != nil {
				log.Fatal(err)
			}
		default:
			log.Fatalf("Unknown command %q", args[0])
		}
	}
}

type Config struct {
	Server server.Config `yaml:"server"`
	Client client.Config `yaml:"client"`
}

func runServer(parent *flag.FlagSet, cfg *server.Config, args []string) (err error) {
	var fs = flag.NewFlagSet(parent.Name()+" server", flag.ContinueOnError)

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage:\n")
		fmt.Fprintf(fs.Output(), "%s [OPTIONS] ARG...\n\nOptions:\n", fs.Name())
		parent.PrintDefaults()
		fs.PrintDefaults()
	}

	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "The server Address")

	if err = fs.Parse(args); err != nil {
		if err.Error() == "flag: help requested" {
			err = nil
		}
		return
	}

	args = fs.Args()
	return server.Serve(cfg, args)
}

var configRe = regexp.MustCompile(`^([^:]+):(.*:\d+)$`)

func runClient(parent *flag.FlagSet, cfg *client.Config, args []string) (err error) {
	fs := flag.NewFlagSet(parent.Name()+" client", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage:\n")
		fmt.Fprintf(fs.Output(), "%s [OPTIONS] ARG...\n\nOptions:\n", fs.Name())
		parent.PrintDefaults()
		fs.PrintDefaults()
	}

	fs.StringVar(&cfg.ServerURL, "server-url", cfg.ServerURL, "The httpdx server url")

	if err = fs.Parse(args); err != nil {
		if err.Error() == "flag: help requested" {
			err = nil
		}
		return
	}

	args = fs.Args()
	if len(args) > 0 {
		cfg.Routes = nil
		for _, arg := range args {
			if m := configRe.FindStringSubmatch(arg); len(m) == 0 {
				return fmt.Errorf("bad argument format")
			} else {
				cfg.Routes = append(cfg.Routes, client.RouteConfig{
					Name:      m[1],
					LocalAddr: m[2],
				})
			}
		}
	}

	return client.Run(cfg)
}

func runCreateConfig(parent *flag.FlagSet, args []string) (err error) {
	var (
		fs         = flag.NewFlagSet(parent.Name()+" create-config", flag.ContinueOnError)
		serverAddr = ":80"
		serverUrl  = "http://127.0.0.1:${PORT}"
		out        = "-"
	)

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage:\n")
		fmt.Fprintf(fs.Output(), "%s [OPTIONS] ARG...\n\nOptions:\n", fs.Name())
		fs.PrintDefaults()
	}

	var port string
	if port, err = portFrom(serverAddr); err != nil {
		return
	}
	serverUrl = strings.ReplaceAll(serverUrl, "${PORT}", port)

	fs.StringVar(&serverUrl, "server-url", serverUrl, "The httpdx server url")
	fs.StringVar(&serverAddr, "server-addr", serverAddr, "The httpdx server addr")
	fs.StringVar(&out, "out", out, "The output file")

	if err = fs.Parse(args); err != nil {
		if err.Error() == "flag: help requested" {
			err = nil
		}
		return
	}

	t, _ := template.New("confile").Parse(`client:
  server_url: "{{.ServerUrl}}"
  routes:
    # - name: ssh
    #  local_addr: :25000
      
server:
  addr: "{{.ServerAddr}}"
  
  # not found HTML file to handles not found error.
  # If not set, uses default not found handler message.
  # not_found: "my_not_found.html"
  
  # if is true, disables not found handles
  # not_found_disabled: false
  
  tcp_sockets:
    # timeouts is in seconds (default is 5s).
    # handshake_timeout: 5
    # dial_timeout: 5
    # write_timeout: 5
    
    # compression_enabled: false
    
    routes:
      # ssh: 
      #  addr: localhost:22
      #  disabled: false
    
  
  http:
    routes:
      # /:
      #  addr: 127.0.0.1:80
      #  disabled: false
        
      # proxify /my-dir as / to destination and pass '/my-dir' 
      # into request header 'path_header' (default is 'X-Forwarded-Prefix')
      # /my-dir:
      #  addr: 127.0.0.1:80
      #  dir: true
      #  path_header: X-Forwarded-Prefix
      #  disabled: false
`)

	var w = os.Stdout

	switch out {
	case "", "-":
	default:
		var f *os.File
		if f, err = os.Create(out); err != nil {
			return
		}
		defer f.Close()
		w = f
	}

	return t.Execute(w, map[string]string{
		"ServerUrl":  serverUrl,
		"ServerAddr": serverAddr,
	})
}

func portFrom(s string) (port string, err error) {
	i := stringsLastIndexByte(s, ':')
	if i == -1 {
		return "", errors.New("serverAddr: not an ip:port")
	}

	port = s[i+1:]
	if len(port) == 0 {
		return "", errors.New("serverAddr: no port")
	}
	return
}

func stringsLastIndexByte(s string, b byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == b {
			return i
		}
	}
	return -1
}
