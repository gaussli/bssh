// Package hosts parses the YAML host list file.
package hosts

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const (
	defaultUser = "root"
	defaultPort = 22
)

// Target is a resolved SSH target.
type Target struct {
	User string
	Host string
	Port int
}

// String returns the user@host:port form.
func (t Target) String() string {
	return fmt.Sprintf("%s@%s:%d", t.User, t.Host, t.Port)
}

type configFile struct {
	Hosts []hostEntry `yaml:"hosts"`
}

type hostEntry struct {
	Host string `yaml:"host"`
	User string `yaml:"user"`
	Port int    `yaml:"port"`
}

// ParseFile reads a YAML file such as:
//
//	hosts:
//	  - host: 192.168.1.1
//	  - host: 192.1.1.1
//	    user: user1
//	    port: 2222
//
// Missing user defaults to root and missing port defaults to 22.
func ParseFile(path string) ([]Target, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var cfg configFile
	decoder := yaml.NewDecoder(f)
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	targets := make([]Target, 0, len(cfg.Hosts))
	for i, h := range cfg.Hosts {
		if h.Host == "" {
			return nil, fmt.Errorf("%s: hosts[%d]: host is required", path, i)
		}
		port := h.Port
		if port == 0 {
			port = defaultPort
		}
		if port < 1 || port > 65535 {
			return nil, fmt.Errorf("%s: hosts[%d]: invalid port %d", path, i, port)
		}
		user := h.User
		if user == "" {
			user = defaultUser
		}
		targets = append(targets, Target{User: user, Host: h.Host, Port: port})
	}
	return targets, nil
}
