package sshclient

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"strconv"
	"sync/atomic"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestRunReusesConnection(t *testing.T) {
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
	defer listener.Close()

	var accepts atomic.Int32
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

	host, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}

	client := &Client{
		auth:            ssh.Password("secret"),
		hostKeyCallback: ssh.InsecureIgnoreHostKey(),
		conns:           make(map[string]*ssh.Client),
	}
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
