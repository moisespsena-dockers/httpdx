package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"

	"github.com/moisespsena-go/httpdx/client"
	"github.com/moisespsena-go/httpdx/server"
	"gopkg.in/yaml.v3"
)

var (
	configFile = filepath.Base(os.Args[0]) + ".yml"
)

func main() {
	flag.StringVar(&configFile, "config", configFile, "YAML configuration file")
	flag.Parse()

	args := flag.Args()

	var (
		data, err = os.ReadFile(configFile)
		cfg       Config
	)

	if err != nil {
		log.Fatal(err)
	}

	if err = yaml.Unmarshal(data, &cfg); err != nil {
		log.Fatal(err)
	}

	if len(args) > 0 {
		if args[0] == "client" {
			args = args[1:]
			if err = runClient(&cfg.Client, args); err != nil {
				log.Fatal(err)
			}
		} else {
			log.Fatalf("Unknown command %q", args[0])
		}
	} else if err = server.Serve(&cfg.Server); err != nil {
		log.Fatal(err)
	}
}

type Config struct {
	Server server.Config `yaml:"server"`
	Client client.Config `yaml:"client"`
}

var configRe = regexp.MustCompile(`^(.+)@(.*:\d+)$`)

func runClient(cfg *client.Config, args []string) (err error) {
	var fs = flag.NewFlagSet("client", flag.ContinueOnError)

	fs.StringVar(&cfg.ServerURL, "server-url", cfg.ServerURL, "The httpdx server url")

	if err = fs.Parse(args); err != nil {
		if err.Error() == "flag: help requested" {
			err = nil
		}
		return
	}

	args = fs.Args()
	if len(args) > 0 {
		cfg.Services = nil
		for _, arg := range args {
			if m := configRe.FindStringSubmatch(arg); len(m) == 0 {
				return fmt.Errorf("bad argument format")
			} else {
				cfg.Services = append(cfg.Services, client.ServiceConfig{
					Name:      m[1],
					LocalAddr: m[2],
				})
			}
		}
	}

	return client.Run(cfg)
}
