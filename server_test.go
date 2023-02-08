package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupDummyServer(t *testing.T) string {
	t.Helper()

	tmpDir := setupDummyEnv(t)

	dummyUpcaseEchoPowerShell := `#!/usr/bin/ruby
require "socket"
File.write('` + tmpDir + `/pid', $$.to_s)
$stdout.sync = true
$stdout << "\xff"
s = $stdin.read(` + fmt.Sprintf("%d", len(repeaterPs1)) + `)
loop do
	# echo
	len = $stdin.read(4)
	data = $stdin.read(len.unpack1("N"))
	exit if data == "fail"
	sleep if data == "stuck"
	$stdout << len + data.upcase
end
`

	os.WriteFile(filepath.Join(tmpDir, "PowerShell.exe"), []byte(dummyUpcaseEchoPowerShell), 0777)

	path := filepath.Join(tmpDir, "tmp.sock")
	s := newServer(path, true)

	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		s.run(ctx)
		close(done)
	}()

	t.Cleanup(func() {
		cancel()
		<-done
	})

	return path
}

func TestServerNormal(t *testing.T) {
	path := setupDummyServer(t)

	sock, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer sock.Close()

	_, err = sock.Write([]byte("\x00\x00\x00\x05hello"))
	if err != nil {
		t.Errorf("failed to communicate: %v", err)
	}

	buf := make([]byte, 4+5)
	n, err := io.ReadFull(sock, buf)
	if err != nil || n != 4+5 || string(buf) != "\x00\x00\x00\x05HELLO" {
		t.Errorf("failed to communicate: %v", err)
	}
}

func TestServerOpenSSHExtension(t *testing.T) {
	path := setupDummyServer(t)

	sock, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer sock.Close()

	_, err = sock.Write([]byte("\x00\x00\x00\x01\x1b"))
	if err != nil {
		t.Errorf("failed to communicate: %v", err)
	}

	buf := make([]byte, 4+1)
	n, err := io.ReadFull(sock, buf)
	if err != nil || n != 5 || string(buf) != "\x00\x00\x00\x01\x06" {
		t.Errorf("failed to communicate: %v", err)
	}
}

func TestServerMultipleAccess(t *testing.T) {
	path := setupDummyServer(t)

	sock1, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer sock1.Close()

	sock2, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer sock2.Close()

	_, err = sock1.Write([]byte("\x00\x00\x00\x05sock1"))
	if err != nil {
		t.Errorf("failed to communicate: %v", err)
	}

	_, err = sock2.Write([]byte("\x00\x00\x00\x05sock2"))
	if err != nil {
		t.Errorf("failed to communicate: %v", err)
	}

	buf := make([]byte, 4+5)
	n, err := io.ReadFull(sock1, buf)
	if err != nil || n != 4+5 || string(buf) != "\x00\x00\x00\x05SOCK1" {
		t.Errorf("failed to communicate: %v", err)
	}

	n, err = io.ReadFull(sock2, buf)
	if err != nil || n != 4+5 || string(buf) != "\x00\x00\x00\x05SOCK2" {
		t.Errorf("failed to communicate: %v", err)
	}
}

func TestServerRestart(t *testing.T) {
	path := setupDummyServer(t)

	sock, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer sock.Close()

	_, err = sock.Write([]byte("\x00\x00\x00\x05Hello"))
	if err != nil {
		t.Errorf("failed to communicate: %v", err)
	}

	buf := make([]byte, 4+5)
	n, err := io.ReadFull(sock, buf)
	if err != nil || n != 4+5 || string(buf) != "\x00\x00\x00\x05HELLO" {
		t.Errorf("failed to communicate: %v", err)
	}

	// stop the dummy PowerShell.exe
	pidStr, err := os.ReadFile(filepath.Join(filepath.Dir(path), "pid"))
	if err != nil {
		t.Fatal("no pid file")
	}
	var pid int
	_, err = fmt.Sscanf(string(pidStr), "%d", &pid)
	if err != nil {
		t.Fatal("pid file is wrong")
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal("no process found")
	}
	proc.Signal(os.Interrupt)
	time.Sleep(500 * time.Millisecond)

	_, err = sock.Write([]byte("\x00\x00\x00\x06Hello2"))
	if err != nil {
		t.Errorf("failed to communicate: %v", err)
	}

	buf = make([]byte, 4+6)
	n, err = io.ReadFull(sock, buf)
	if err != nil || n != 4+6 || string(buf) != "\x00\x00\x00\x06HELLO2" {
		t.Errorf("failed to communicate: %v", err)
	}
}

func TestServerFail(t *testing.T) {
	path := setupDummyServer(t)

	sock, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer sock.Close()

	_, err = sock.Write([]byte("\x00\x00\x00\x04fail"))
	if err != nil {
		t.Errorf("failed to communicate: %v", err)
	}

	buf := make([]byte, 1)
	_, err = io.ReadFull(sock, buf)
	if err != io.EOF {
		t.Errorf("it should fail with EOF: %v", err)
	}
}

func TestServerStuck(t *testing.T) {
	path := setupDummyServer(t)

	sock, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer sock.Close()

	readTimeLimitBackup := readTimeLimit
	readTimeLimit = 500 * time.Millisecond
	defer func() {
		readTimeLimit = readTimeLimitBackup
	}()

	_, err = sock.Write([]byte("\x00\x00\x00\x05stuck"))
	if err != nil {
		t.Errorf("failed to communicate: %v", err)
	}

	buf := make([]byte, 1)
	_, err = io.ReadFull(sock, buf)
	if err != io.EOF {
		t.Errorf("it should fail with EOF: %v", err)
	}
}
