package main

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"time"
)

//go:embed repeater.ps1
var repeaterPs1 string

type repeater struct {
	in  io.WriteCloser
	out io.Reader
	cmd *exec.Cmd
}

var waitTimes []time.Duration = []time.Duration{
	3 * time.Second,
	6 * time.Second,
	10 * time.Second,
}

// invoke PowerShell.exe and run
func newRepeater(ctx context.Context) (*repeater, error) {
	for i, limit := range waitTimes {
		log.Printf("invoking [W] in PowerShell.exe%s", trial(i))

		cmd := exec.Command("PowerShell.exe", "-Command", "-")
		in, err := cmd.StdinPipe()
		if err != nil {
			continue
		}
		out, err := cmd.StdoutPipe()
		if err != nil {
			continue
		}
		cmd.Stderr = logOutput

		err = cmd.Start()
		if err != nil {
			log.Printf("failed to invoke [W]: %s", err)
			continue
		}

		// write the source code
		_, err = io.WriteString(in, repeaterPs1)
		if err != nil {
			log.Printf("failed to give [W] the source code: %s", err)
			terminate(cmd)
			continue
		}

		done := make(chan bool)

		// wait for the process start up
		go func() {
			// the process should output "\xff" if it starts successfully
			buf := make([]byte, 1)
			n, err := out.Read(buf)
			done <- err == nil && n == 1 && buf[0] == 0xff
		}()

		select {
		case ok := <-done:
			if ok {
				log.Printf("[W] invoked successfully")
				return &repeater{in, out, cmd}, nil
			}
		case <-time.After(limit):
		}

		log.Printf("PowerShell.exe does not respond in %v", limit)
		terminate(cmd)
	}

	return nil, fmt.Errorf("failed to invoke PowerShell.exe %d times; give up", len(waitTimes))
}

func terminate(cmd *exec.Cmd) {
	err := cmd.Process.Kill()
	if err != nil {
		log.Fatal(err)
	}
	cmd.Wait() //nolint:errcheck
}

func (rep *repeater) terminate() {
	rep.in.Close()
	terminate(rep.cmd)
}

func getWinSshVersion() string {
	for i, limit := range waitTimes {
		ctx, cancel := context.WithTimeout(context.Background(), limit)
		defer cancel()

		log.Printf("check the version of ssh.exe%s", trial(i))

		cmd := exec.CommandContext(ctx, "ssh.exe", "-V")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()

		if err != nil {
			log.Printf("failed to invoke ssh.exe: %s", err)
			continue
		}

		version := strings.TrimSuffix(stderr.String(), "\r\n")

		log.Printf("the version of ssh.exe: %#v", version)
		return version
	}

	log.Printf("failed to check the version of ssh.exe")

	return ""
}

func trial(i int) string {
	if i == 0 {
		return ""
	} else {
		return fmt.Sprintf(" (trial %d/%d)", i+1, len(waitTimes))
	}
}
