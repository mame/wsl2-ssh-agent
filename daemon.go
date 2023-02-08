package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

// check if the file descripter 3 is open
func checkDaemonMode() bool {
	_, err := unix.FcntlInt(3, unix.F_GETFD, 0)

	parent := err != nil
	if !parent {
		syscall.CloseOnExec(3)
	}

	return parent
}

// invoke itself as a child process
func startDaemonizing(args ...string) {
	r, w, err := os.Pipe()
	if err != nil {
		log.Fatal(err)
	}

	exe, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}

	// invoke itself
	cmd := exec.Command(exe, args...)
	cmd.ExtraFiles = []*os.File{w}
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	err = cmd.Start()
	if err != nil {
		log.Fatal(err)
	}

	// wait for the child process
	w.Close()
	buf, err := io.ReadAll(r)
	if err != nil {
		log.Fatal(err)
	}
	r.Close()

	// output the script
	fmt.Println(string(buf))

	os.Stdin.Close()

	os.Exit(0)
}

// say ok to the parent process
func completeDaemonizing(output string) {
	pipe := os.NewFile(3, "pipe")
	_, err := pipe.WriteString(output)
	if err != nil {
		log.Fatal(err)
	}
	pipe.Close()

	os.Stdin.Close()
	os.Stdout.Close()
	os.Stderr.Close()

	_, err = syscall.Setsid()
	if err != nil {
		log.Fatal(fmt.Errorf("failed to setsid: %s", err))
	}
}
