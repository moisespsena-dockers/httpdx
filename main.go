package main

import (
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/moisespsena-go/httpdx/client"
	"github.com/moisespsena-go/httpdx/server"
	"gopkg.in/yaml.v3"
)

var (
	configFile = filepath.Base(os.Args[0]) + ".yml"
	buildTime  string
)

func main() {
	fs := flag.NewFlagSet(filepath.Base(os.Args[0]), flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage:\n")
		fmt.Fprintf(fs.Output(), "%s [COMMAND] [OPTIONS]\n\n", fs.Name())
		fmt.Fprintf(fs.Output(), "Available commands:\n"+
			"  server (default): run as server.\n"+
			"  client:           run as client.\n"+
			"  create-config:    create config file.\n"+
			"  info:             print program information.\n\n")
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
		args = fs.Args()
		cfg  Config
		err  error

		readConfig = func() {
			var data []byte
			if data, err = os.ReadFile(configFile); err == nil {
				err = yaml.Unmarshal(data, &cfg)
			}
		}
	)

	if l := len(args); l == 0 || args[0] == "server" {
		if readConfig(); err == nil {
			err = runServer(fs, &cfg.Server, nil)
		}
	} else if l > 0 {
		switch args[0] {
		case "client":
			if readConfig(); err == nil {
				err = runClient(fs, &cfg.Client, args[1:])
			}
		case "create-config":
			err = runCreateConfig(fs, args[1:])
		case "info":
			err = runAbout(fs, args[1:])
		default:
			err = fmt.Errorf("Unknown command %q", args[0])
		}
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
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
	return server.Serve(cfg)
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
				cfg.Routes = append(cfg.Routes, &client.RouteConfig{
					Name:      m[1],
					LocalAddr: m[2],
				})
			}
		}
	}

	return client.Run(cfg)
}

//go:embed config_template.yml
var configTemplate string

func runCreateConfig(parent *flag.FlagSet, args []string) (err error) {
	var (
		fs         = flag.NewFlagSet(parent.Name()+" create-config", flag.ContinueOnError)
		serverAddr = ":80"
		serverUrl  = "http://127.0.0.1:PORT"
		out        = "-"
	)

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage:\n")
		fmt.Fprintf(fs.Output(), "%s [OPTIONS] ARG...\n\nOptions:\n", fs.Name())
		fs.PrintDefaults()
	}

	fs.StringVar(&serverUrl, "server-url", serverUrl, "The httpdx server url")
	fs.StringVar(&serverAddr, "server-addr", serverAddr, "The httpdx server addr")
	fs.StringVar(&out, "out", out, "The output file")

	if err = fs.Parse(args); err != nil {
		if err.Error() == "flag: help requested" {
			err = nil
		}
		return
	}

	var port string

	if i2, _ := strconv.Atoi(serverAddr); i2 > 0 {
		port = serverAddr
		serverAddr = ":" + serverAddr
	} else if port, err = portFrom(serverAddr); err != nil {
		return
	}
	serverUrl = strings.ReplaceAll(serverUrl, "PORT", port)

	t, _ := template.New("confile").Parse(configTemplate)

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

func runAbout(parent *flag.FlagSet, args []string) (err error) {
	var (
		fs     = flag.NewFlagSet(parent.Name()+" info", flag.ContinueOnError)
		format = "Commit Id: {{.CommitID}}\n" +
			"Commit Time: {{.CommitTime}}\n" +
			"Build Time: {{.BuildTime}}{{if .CommitModified}} (modified){{end}}\n" +
			"{{if .GoVarsKeys}}" +
			"{{$goVars := .GoVars}}" +
			"Go Variables:\n" +
			"{{range .GoVarsKeys}}" +
			"  {{.}}={{index $goVars .}}\n" +
			"{{end}}" +
			"{{end}}" +
			"Project Page: {{.SiteUrl}}"
	)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage:\n")
		fmt.Fprintf(fs.Output(), "%s [OPTIONS] ARG...\n\nOptions:\n", fs.Name())
		parent.PrintDefaults()
		fs.PrintDefaults()
	}

	fs.StringVar(&format, "format", format, "print version info by go template format."+
		"\n")

	if err = fs.Parse(args); err != nil {
		if err.Error() == "flag: help requested" {
			err = nil
		}
		return
	}

	var (
		t                    *template.Template
		commitID, commitTime string
		modified             bool
		goVarsKeys           []string
		goVars               = map[string]string{}
	)

	if t, err = template.New("info").Parse(format); err != nil {
		return
	}

	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs":
			case "vcs.time":
				commitTime = setting.Value
			case "vcs.revision":
				commitID = setting.Value
			case "vcs.modified":
				modified = true
			default:
				goVarsKeys = append(goVarsKeys, setting.Key)
				goVars[setting.Key] = setting.Value
			}
		}
	}

	sort.Strings(goVarsKeys)

	if buildTime != "" {
		if i, _ := strconv.Atoi(buildTime); i > 0 {
			t := time.Unix(int64(i), 0)
			buildTime = t.UTC().Format("2006-01-02T15:04:05Z")
		}
	}

	err = t.Execute(os.Stdout, map[string]any{
		"CommitID":       commitID,
		"CommitTime":     commitTime,
		"CommitModified": modified,
		"BuildTime":      buildTime,
		"GoVars":         goVars,
		"GoVarsKeys":     goVarsKeys,
		"SiteUrl":        "https://github.com/moisespsena-go/httpdx",
	})
	if err == nil {
		println()
	}
	return
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
