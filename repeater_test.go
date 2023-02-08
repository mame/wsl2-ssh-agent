package main

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const dummyBrokenPowerShell = `#!/usr/bin/ruby
$stdin.read
`

const dummyEchoPowerShell = `#!/usr/bin/ruby
$stdout.sync = true
$stdout << "\xff"
loop do
  $stdout << $stdin.getc
end
`

const dummySsh = `#!/usr/bin/ruby
$stderr << "Hello\r\n"
`

func setupDummyEnv(t *testing.T) (string, func()) {
	log.SetOutput(io.Discard)
	tmpDir := t.TempDir()
	pathBackup := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir)
	waitTimesBackup := waitTimes
	waitTimes = []time.Duration{0 * time.Second, 500 * time.Millisecond}

	return tmpDir, func() {
		waitTimes = waitTimesBackup
		os.Setenv("PATH", pathBackup)
	}
}

func TestRepeaterNoPowerShell(t *testing.T) {
	_, teardown := setupDummyEnv(t)
	defer teardown()

	_, err := newRepeater(context.Background())
	if err.Error() != "failed to invoke PowerShell.exe 2 times; give up" {
		t.Errorf("should fail")
	}
}

func TestRepeaterBrokenPowerShell(t *testing.T) {
	tmpDir, teardown := setupDummyEnv(t)
	defer teardown()

	os.WriteFile(filepath.Join(tmpDir, "PowerShell.exe"), []byte(dummyBrokenPowerShell), 0777)
	_, err := newRepeater(context.Background())
	if err.Error() != "failed to invoke PowerShell.exe 2 times; give up" {
		t.Errorf("should fail")
	}
}

func TestRepeaterNormal(t *testing.T) {
	tmpDir, teardown := setupDummyEnv(t)
	defer teardown()

	os.WriteFile(filepath.Join(tmpDir, "PowerShell.exe"), []byte(dummyEchoPowerShell), 0777)

	rep, err := newRepeater(context.Background())
	if err != nil {
		t.Errorf("failed: %s", err)
	}

	buf := make([]byte, len(repeaterPs1))
	io.ReadFull(rep.out, buf)
	if string(buf) != repeaterPs1 {
		t.Errorf("does not work")
	}

	rep.in.Write([]byte("Hello"))
	buf = make([]byte, 5)
	io.ReadFull(rep.out, buf)
	if string(buf) != "Hello" {
		t.Errorf("does not work")
	}

	rep.terminate()
}

func TestSshVersionNoSsh(t *testing.T) {
	_, teardown := setupDummyEnv(t)
	defer teardown()

	s := getWinSshVersion()
	if s != "" {
		t.Errorf("getWinSshVersion should fail")
	}
}

func TestSshVersionNormal(t *testing.T) {
	tmpDir, teardown := setupDummyEnv(t)
	defer teardown()

	os.WriteFile(filepath.Join(tmpDir, "ssh.exe"), []byte(dummySsh), 0777)

	s := getWinSshVersion()
	if s != "Hello" {
		t.Errorf("getWinSshVersion does not work well: %#v", s)
	}
}
