package sshclient

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"strconv"
	"sync/atomic"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/gaussli/bssh/internal/hosts"
)

func newTestSSHServer(t *testing.T) (string, int, *atomic.Int32) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}

	serverConfig := &ssh.ServerConfig{
		PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) {
			return nil, nil
		},
	}
	serverConfig.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	accepts := new(atomic.Int32)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			accepts.Add(1)
			go serveTestSSH(conn, serverConfig)
		}
	}()
	t.Cleanup(func() { _ = listener.Close() })

	host, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	return host, port, accepts
}

func newTestClient() *Client {
	return &Client{
		auth:            ssh.Password("secret"),
		hostKeyCallback: ssh.InsecureIgnoreHostKey(),
		conns:           make(map[string]*ssh.Client),
	}
}

func TestRunReusesConnection(t *testing.T) {
	host, port, accepts := newTestSSHServer(t)
	client := newTestClient()
	defer client.Close()

	for _, command := range []string{"first", "second", "third"} {
		out, err := client.Run("tester", host, port, command)
		if err != nil {
			t.Fatalf("run %q: %v", command, err)
		}
		if out != command {
			t.Fatalf("run %q returned %q", command, out)
		}
	}
	if got := accepts.Load(); got != 1 {
		t.Fatalf("server accepted %d connections, want 1", got)
	}

	client.Close()
	if len(client.conns) != 0 {
		t.Fatalf("Close left %d cached connections", len(client.conns))
	}
}

func TestConnectPrewarmsConnections(t *testing.T) {
	host1, port1, accepts1 := newTestSSHServer(t)
	host2, port2, accepts2 := newTestSSHServer(t)

	targets := []hosts.Target{
		{User: "tester", Host: host1, Port: port1},
		{User: "tester", Host: host2, Port: port2},
	}
	client := newTestClient()
	defer client.Close()

	done := 0
	errs := client.Connect(targets, func(_ int, _ error) {
		done++
	})
	for i, err := range errs {
		if err != nil {
			t.Fatalf("connect targets[%d]: %v", i, err)
		}
	}
	if done != len(targets) {
		t.Fatalf("onDone called %d times, want %d", done, len(targets))
	}
	if len(client.conns) != len(targets) {
		t.Fatalf("cached %d connections, want %d", len(client.conns), len(targets))
	}
	if accepts1.Load() != 1 || accepts2.Load() != 1 {
		t.Fatalf("Connect opened %d/%d connections, want 1/1", accepts1.Load(), accepts2.Load())
	}

	for _, target := range targets {
		out, err := client.Run(target.User, target.Host, target.Port, "hostname")
		if err != nil {
			t.Fatalf("run on %s: %v", target, err)
		}
		if out != "hostname" {
			t.Fatalf("run on %s returned %q", target, out)
		}
	}
	if accepts1.Load() != 1 || accepts2.Load() != 1 {
		t.Fatalf("commands opened new connections (%d/%d)", accepts1.Load(), accepts2.Load())
	}
}

// serveTestSSH accepts session channels and echoes the exec command on stdout.
func serveTestSSH(conn net.Conn, config *ssh.ServerConfig) {
	serverConn, channels, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		_ = conn.Close()
		return
	}
	defer serverConn.Close()
	go ssh.DiscardRequests(requests)

	for newChannel := range channels {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "session only")
			continue
		}
		channel, channelRequests, err := newChannel.Accept()
		if err != nil {
			return
		}
		go func() {
			defer channel.Close()
			for req := range channelRequests {
				if req.Type != "exec" {
					continue
				}
				var payload struct{ Command string }
				_ = ssh.Unmarshal(req.Payload, &payload)
				_ = req.Reply(true, nil)
				_, _ = channel.Write([]byte(payload.Command + "\n"))
				_, _ = channel.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
				return
			}
		}()
	}
}
