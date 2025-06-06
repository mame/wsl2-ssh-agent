package main

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"log"
	"os/exec"
	"time"
)

//go:embed repeater.ps1
var repeaterPs1 string

type repeater struct {
	in  io.WriteCloser
	out io.Reader
	cmd *exec.Cmd
}

var waitTimes = []time.Duration{
	3 * time.Second,
	6 * time.Second,
	10 * time.Second,
}

// invoke PowerShell.exe and run
func newRepeater(ctx context.Context, powershell string, pipename string) (*repeater, error) {
	for i, limit := range waitTimes {
		log.Printf("invoking [W] in PowerShell.exe%s", trial(i))

		cmd := exec.Command(powershell, "-Command", "-")
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
			// ignore any output until we got "\xff"
			buf := make([]byte, 1)
			for {
				n, err := out.Read(buf)
				if err != nil {
					done <- false
					return
				}
				if n == 1 && buf[0] == 0xff {
					done <- true
					return
				}
			}
		}()

		select {
		case ok := <-done:
			if ok {
				log.Printf("[W] invoked successfully")

				buf := make([]byte, 4)
				buf[0] = byte((len(pipename) >> 24) & 0xff)
				buf[1] = byte((len(pipename) >> 16) & 0xff)
				buf[2] = byte((len(pipename) >> 8) & 0xff)
				buf[3] = byte(len(pipename) & 0xff)
				_, err = io.WriteString(in, string(buf)+pipename)
				if err != nil {
					log.Printf("failed to give [W] the pipe name: %s", err)
					terminate(cmd)
					continue
				}

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

func trial(i int) string {
	if i == 0 {
		return ""
	} else {
		return fmt.Sprintf(" (trial %d/%d)", i+1, len(waitTimes))
	}
}
