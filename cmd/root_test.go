package cmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gaussli/bssh/internal/hosts"
)

type fakeClient struct {
	connectErrs []error
	events      *[]string
}

func (f *fakeClient) Connect(_ []hosts.Target, onDone func(index int, err error)) []error {
	*f.events = append(*f.events, "connect:start")
	for i, err := range f.connectErrs {
		if onDone != nil {
			onDone(i, err)
		}
	}
	*f.events = append(*f.events, "connect:done")
	return f.connectErrs
}

func (f *fakeClient) Run(_ string, _ string, _ int, command string) (string, error) {
	*f.events = append(*f.events, "run:"+command)
	return "ok", nil
}

type recordingReader struct {
	src    io.Reader
	events *[]string
	read   bool
}

func (r *recordingReader) Read(p []byte) (int, error) {
	if !r.read {
		r.read = true
		*r.events = append(*r.events, "input:read")
	}
	return r.src.Read(p)
}

func TestConnectHostsReportsProgress(t *testing.T) {
	targets := []hosts.Target{
		{User: "root", Host: "192.168.1.1", Port: 22},
		{User: "deploy", Host: "192.168.1.2", Port: 2222},
	}
	events := []string{}
	client := &fakeClient{connectErrs: []error{nil, errors.New("timeout")}, events: &events}
	var out bytes.Buffer

	ready, err := connectHosts(client, targets, &out)
	if err != nil {
		t.Fatalf("connectHosts: %v", err)
	}
	if len(ready) != 1 || ready[0] != targets[0] {
		t.Fatalf("ready = %v, want only the first target", ready)
	}

	got := out.String()
	for _, want := range []string{
		"正在连接 root@192.168.1.1:22 ...",
		"正在连接 deploy@192.168.1.2:2222 ...",
		"已连接 root@192.168.1.1:22",
		"连接失败 deploy@192.168.1.2:2222: timeout",
		"已连接 1/2 台主机，可以输入命令（exit 退出）",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("progress output missing %q:\n%s", want, got)
		}
	}
}

func TestConnectHostsFailsWhenNoHostConnected(t *testing.T) {
	targets := []hosts.Target{{User: "root", Host: "10.0.0.1", Port: 22}}
	events := []string{}
	client := &fakeClient{connectErrs: []error{errors.New("refused")}, events: &events}
	var out bytes.Buffer

	if _, err := connectHosts(client, targets, &out); err == nil {
		t.Fatal("expected an error when no host connects")
	}
	if strings.Contains(out.String(), "可以输入命令") {
		t.Errorf("should not invite input when no host connects:\n%s", out.String())
	}
}

func TestRunWithConnectsBeforeReadingInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts.yaml")
	if err := os.WriteFile(path, []byte("hosts:\n  - host: 192.168.1.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	events := []string{}
	client := &fakeClient{connectErrs: []error{nil}, events: &events}
	in := &recordingReader{src: strings.NewReader("hostname\nexit\n"), events: &events}
	var out, errOut bytes.Buffer
	if err := runWith(options{file: path}, client, in, &out, &errOut); err != nil {
		t.Fatalf("runWith: %v", err)
	}

	indexOf := func(name string) int {
		for i, event := range events {
			if event == name {
				return i
			}
		}
		return -1
	}
	connectDone := indexOf("connect:done")
	inputRead := indexOf("input:read")
	commandRun := indexOf("run:hostname")
	if connectDone < 0 || inputRead < 0 || commandRun < 0 {
		t.Fatalf("unexpected event sequence: %v", events)
	}
	if connectDone >= inputRead {
		t.Fatalf("input was read before connections finished: %v", events)
	}
	if commandRun <= inputRead {
		t.Fatalf("command ran before input was read: %v", events)
	}
	if !strings.Contains(out.String(), "root@192.168.1.1:22\tok") {
		t.Errorf("unexpected command output: %q", out.String())
	}
}
