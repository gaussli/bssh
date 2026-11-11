package hosts

import (
	"os"
	"path/filepath"
	"testing"
)

func writeHostsFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hosts.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseFile(t *testing.T) {
	path := writeHostsFile(t, `hosts:
- ip: 192.168.1.1
- ip: 192.1.1.1
  port: 22
  user: user1
`)
	targets, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []Target{
		{User: "root", Host: "192.168.1.1", Port: 22},
		{User: "user1", Host: "192.1.1.1", Port: 22},
	}
	if len(targets) != len(want) {
		t.Fatalf("got %d targets, want %d: %v", len(targets), len(want), targets)
	}
	for i := range want {
		if targets[i] != want[i] {
			t.Errorf("target %d = %v, want %v", i, targets[i], want[i])
		}
	}
}

func TestParseFileCustomPort(t *testing.T) {
	path := writeHostsFile(t, "hosts:\n  - ip: 10.0.0.1\n    port: 2222\n")
	targets, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Port != 2222 {
		t.Fatalf("unexpected targets: %v", targets)
	}
}

func TestParseFileRejectsInvalidInput(t *testing.T) {
	cases := map[string]string{
		"missing ip":    "hosts:\n- user: root\n",
		"invalid port":  "hosts:\n- ip: 10.0.0.1\n  port: 70000\n",
		"unknown field": "hosts:\n- ip: 10.0.0.1\n  prot: 22\n",
		"old format":    "root@192.168.1.1\n",
		"broken yaml":   "hosts: [\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseFile(writeHostsFile(t, content)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
