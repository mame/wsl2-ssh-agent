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
$stdout << "Warning: Some dummy ramdom warming messages ... Dummy ...\n"
$stdout << "\xff"
loop do
  $stdout << $stdin.getc
end
`

func setupDummyEnv(t *testing.T) string {
	t.Helper()
	log.SetOutput(io.Discard)
	tmpDir := t.TempDir()
	t.Setenv("PATH", tmpDir)
	waitTimesBackup := waitTimes
	waitTimes = []time.Duration{0 * time.Second, 500 * time.Millisecond, 3 * time.Second}

	t.Cleanup(func() {
		waitTimes = waitTimesBackup
	})
	return tmpDir
}

func TestRepeaterNoPowerShell(t *testing.T) {
	setupDummyEnv(t)

	_, err := newRepeater(context.Background(), "/dummy/powershell.exe", "dummy-pipe-name")
	if err == nil || err.Error() != "failed to invoke PowerShell.exe 3 times; give up" {
		t.Errorf("should fail")
	}
}

func TestRepeaterBrokenPowerShell(t *testing.T) {
	tmpDir := setupDummyEnv(t)

	err := os.WriteFile(filepath.Join(tmpDir, "powershell.exe"), []byte(dummyBrokenPowerShell), 0777)
	if err != nil {
		t.Fatal(err)
	}
	_, err = newRepeater(context.Background(), powershellPath(), "dummy-pipe-name")
	if err == nil || err.Error() != "failed to invoke PowerShell.exe 3 times; give up" {
		t.Errorf("should fail")
	}
}

func TestRepeaterNormal(t *testing.T) {
	tmpDir := setupDummyEnv(t)

	err := os.WriteFile(filepath.Join(tmpDir, "powershell.exe"), []byte(dummyEchoPowerShell), 0777)
	if err != nil {
		t.Fatal(err)
	}

	rep, err := newRepeater(context.Background(), powershellPath(), "dummy-pipe-name")
	if err != nil {
		t.Errorf("failed: %s", err)
	}

	buf := make([]byte, len(repeaterPs1))
	_, err = io.ReadFull(rep.out, buf)
	if err != nil || string(buf) != repeaterPs1 {
		t.Errorf("does not work")
	}

	buf = make([]byte, 19)
	_, err = io.ReadFull(rep.out, buf)
	if err != nil || string(buf) != "\x00\x00\x00\x0fdummy-pipe-name" {
		t.Errorf("does not work: %s", string(buf))
	}

	_, err = rep.in.Write([]byte("Hello"))
	if err != nil {
		t.Fatal(err)
	}
	buf = make([]byte, 5)
	_, err = io.ReadFull(rep.out, buf)
	if err != nil || string(buf) != "Hello" {
		t.Errorf("does not work")
	}

	rep.terminate()
}
