/*
  Copyright (c) 2012-2013 José Carlos Nieto, http://xiam.menteslibres.org/

  Permission is hereby granted, free of charge, to any person obtaining
  a copy of this software and associated documentation files (the
  "Software"), to deal in the Software without restriction, including
  without limitation the rights to use, copy, modify, merge, publish,
  distribute, sublicense, and/or sell copies of the Software, and to
  permit persons to whom the Software is furnished to do so, subject to
  the following conditions:

  The above copyright notice and this permission notice shall be
  included in all copies or substantial portions of the Software.

  THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
  EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
  MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
  NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
  LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
  OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
  WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/

package main

import (
	"flag"
	"fmt"
	"github.com/gosexy/cli"
	"github.com/gosexy/to"
	"github.com/gosexy/yaml"
	"github.com/xiam/luminos/host"
	"log"
	"net"
	"net/http"
	"net/http/fcgi"
	"os"
	"strings"
)

// Default values
const DEFAULT_SETTINGS_FILE = "./settings.yaml"
const DEFAULT_SERVER_DOMAIN = "unix"
const DEFAULT_SERVER_PROTOCOL = "tcp"

// Global settings.
var settings *yaml.Yaml

// Host map.
var hosts map[string]*host.Host

// Settings
var flagConf = flag.String("c", "./settings.yaml", "Path to the settings.yaml file.")

func init() {

	cli.Register("run", cli.Entry{
		Name:        "run",
		Description: "Runs a luminos server.",
		Arguments:   []string{"c"},
		Command:     &runCommand{},
	})

	hosts = make(map[string]*host.Host)
}

type server struct {
}

type runCommand struct {
}

// Dispatches a request and returns the appropriate host.
func route(req *http.Request) *host.Host {

	name := req.Host

	if strings.Contains(name, ":") {
		name = name[0:strings.Index(name, ":")]
	}

	path := name + req.URL.Path

	// Searching for best match for host.
	match := ""

	for key, _ := range hosts {
		lkey := len(key)
		if lkey > len(match) {
			if path[0:lkey] == key {
				match = key
			}
		}
	}

	if match == "" {
		log.Printf("Could not match any host: %s, falling back to default.\n", req.Host)
		match = "default"
	}

	if _, ok := hosts[match]; ok == true {
		return hosts[match]
	}

	log.Printf("Request for unknown host: %s\n", req.Host)

	return nil

}

// Routes a request and lets the host handle it.
func (self server) ServeHTTP(wri http.ResponseWriter, req *http.Request) {
	r := route(req)
	if r != nil {
		r.ServeHTTP(wri, req)
	} else {
		log.Printf("Failed to serve host %s.\n", req.Host)
	}
}

func (self *runCommand) Execute() error {

	// Default settings file.
	settingsFile := DEFAULT_SETTINGS_FILE

	if *flagConf != "" {
		// Overriding settings file.
		settingsFile = *flagConf
	}

	stat, err := os.Stat(settingsFile)

	if err != nil {
		return fmt.Errorf("Error while opening %s: %s", settingsFile, err.Error())
	}

	if stat != nil {

		if stat.IsDir() == true {

			return fmt.Errorf("Could not open %s: it's a directory!", settingsFile)

		} else {

			// Trying to read settings from file.
			settings, err = yaml.Open(settingsFile)

			if err != nil {
				return fmt.Errorf("Error while reading settings file %s: %s", settingsFile, err.Error())
			}

			serverType := to.String(settings.Get("server", "type"))

			domain := DEFAULT_SERVER_DOMAIN
			address := to.String(settings.Get("server", "socket"))

			if address == "" {
				domain = DEFAULT_SERVER_PROTOCOL
				address = fmt.Sprintf("%s:%d", to.String(settings.Get("server", "bind")), to.Int(settings.Get("server", "port")))
			}

			listener, err := net.Listen(domain, address)

			if err != nil {
				return err
			}

			// Loading and verifying host entries
			entries := to.Map(settings.Get("hosts"))

			for name, _ := range entries {
				path := to.String(entries[name])

				info, err := os.Stat(path)
				if err != nil {
					return fmt.Errorf("Failed to validate host %s: %s.", name, err.Error())
				}
				if info.IsDir() == false {
					return fmt.Errorf("Host %s does not point to a directory.", name)
				}
				// Just allocating map key.
				hosts[name], err = host.New(name, path)

				if err != nil {
					return fmt.Errorf("Failed to initialize host %s: %s.", name, err.Error())
				}
			}

			if _, ok := entries["default"]; ok == false {
				log.Printf("Warning: default host was not provided.")
			}

			defer listener.Close()

			switch serverType {
			case "fastcgi":
				if err == nil {
					log.Printf("Starting FastCGI server. Listening at %s.", address)
					fcgi.Serve(listener, &server{})
				} else {
					return fmt.Errorf("Failed to start FastCGI server: %s", err.Error())
				}
			case "standalone":
				if err == nil {
					log.Printf("Starting HTTP server. Listening at %s.", address)
					http.Serve(listener, &server{})
				} else {
					return fmt.Errorf("Failed to start HTTP server: %s", err.Error())
				}
			default:
				return fmt.Errorf("Unknown server type: %s", serverType)
			}

		}
	} else {
		return fmt.Errorf("Could not load settings file: %s.", settingsFile)
	}

	return nil
}
