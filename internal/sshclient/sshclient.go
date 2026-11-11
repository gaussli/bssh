// Package sshclient runs a command on remote hosts over SSH.
package sshclient

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const connectTimeout = 10 * time.Second

// Client holds the resolved auth method, host key policy and cached connections.
// It is not safe for concurrent use; commands are executed serially.
type Client struct {
	auth            ssh.AuthMethod
	hostKeyCallback ssh.HostKeyCallback
	conns           map[string]*ssh.Client
}

// New resolves the private key and host key policy once, before any dialing.
func New() (*Client, error) {
	auth, err := publicKeyAuth()
	if err != nil {
		return nil, err
	}
	callback, err := newHostKeyCallback()
	if err != nil {
		return nil, err
	}
	return &Client{
		auth:            auth,
		hostKeyCallback: callback,
		conns:           make(map[string]*ssh.Client),
	}, nil
}

// Close closes every cached SSH connection.
func (c *Client) Close() {
	for key, conn := range c.conns {
		_ = conn.Close()
		delete(c.conns, key)
	}
}

// Run executes command on one host over a cached connection and returns its
// trimmed stdout. An ssh.Session is single-use, so a fresh session is opened
// for each command while the underlying connection is reused.
func (c *Client) Run(user, host string, port int, command string) (string, error) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	session, err := c.newSession(user, addr, user+"@"+addr)
	if err != nil {
		return "", err
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	if err := session.Run(command); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("run %q: %w: %s", command, err, msg)
		}
		return "", fmt.Errorf("run %q: %w", command, err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (c *Client) newSession(user, addr, key string) (*ssh.Session, error) {
	conn, ok := c.conns[key]
	if !ok {
		var err error
		conn, err = c.dial(user, addr)
		if err != nil {
			return nil, err
		}
		c.conns[key] = conn
	}
	session, err := conn.NewSession()
	if err == nil {
		return session, nil
	}
	// A cached connection may have gone stale; drop it and retry once.
	_ = conn.Close()
	delete(c.conns, key)
	conn, err = c.dial(user, addr)
	if err != nil {
		return nil, err
	}
	c.conns[key] = conn
	session, err = conn.NewSession()
	if err != nil {
		return nil, fmt.Errorf("open session on %s: %w", key, err)
	}
	return session, nil
}

func (c *Client) dial(user, addr string) (*ssh.Client, error) {
	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{c.auth},
		HostKeyCallback: c.hostKeyCallback,
		Timeout:         connectTimeout,
	}
	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", addr, err)
	}
	return conn, nil
}

func publicKeyAuth() (ssh.AuthMethod, error) {
	path, err := defaultIdentity()
	if err != nil {
		return nil, err
	}
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read private key %s: %w", path, err)
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("parse private key %s: %w (encrypted keys are not supported)", path, err)
	}
	return ssh.PublicKeys(signer), nil
}

func defaultIdentity() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	dir := filepath.Join(home, ".ssh")
	for _, name := range []string{"id_ed25519", "id_rsa", "id_ecdsa"} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no private key in %s (tried id_ed25519, id_rsa, id_ecdsa); place one there", dir)
}

func newHostKeyCallback() (ssh.HostKeyCallback, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("find home directory: %w", err)
	}
	path := filepath.Join(home, ".ssh", "known_hosts")
	callback, err := knownhosts.New(path)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w (add the host key first, e.g. with ssh-keyscan)", path, err)
	}
	return callback, nil
}
