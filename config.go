package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
)

type config struct {
	socketPath     string
	powershellPath string
	foreground     bool
	verbose        bool
	stop           bool
	logFile        string
	version        bool
}

var version = "(development version)"
var logOutput io.Writer

func defaultSocketPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	return filepath.Join(home, ".ssh", "wsl2-ssh-agent.sock")
}

func powershellPath() string {
	path, err := exec.LookPath("powershell.exe")
	if err != nil {
		path := "/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe"
		_, err := os.Stat(path)
		if err == nil {
			return path
		} else {
			return ""
		}
	}
	return path
}

func newConfig() *config {
	c := &config{}

	flag.StringVar(&c.socketPath, "socket", defaultSocketPath(), "a path of UNIX domain socket to listen")
	flag.StringVar(&c.powershellPath, "powershell-path", powershellPath(), "a path of Windows PowerShell (powershell.exe)")
	flag.BoolVar(&c.foreground, "foreground", false, "run in foreground mode")
	flag.BoolVar(&c.verbose, "verbose", false, "verbose mode")
	flag.StringVar(&c.logFile, "log", "", "a file path to write the log")
	flag.BoolVar(&c.stop, "stop", false, "stop the daemon and exit")
	flag.BoolVar(&c.version, "version", false, "print version and exit")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: wsl2-ssh-agent\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if c.powershellPath == "" {
		fmt.Printf("powershell.exe not found, use the -powershell-path to customize the path.\n")
		os.Exit(1)
	}

	return c
}

func (c *config) start() context.Context {
	if c.version {
		fmt.Printf("wsl2-ssh-agent %s\n", version)
		os.Exit(0)
	}

	// check if this process is a child or not
	parent := checkDaemonMode()

	// script output
	output := fmt.Sprintf("SSH_AUTH_SOCK=%s; export SSH_AUTH_SOCK;", c.socketPath)

	// set up the log file
	c.setupLogFile()

	// check if wsl2-ssl-agent is already running
	serverPid := findRunningServerPid(c.socketPath)

	// --stop option
	if c.stop {
		if serverPid == -1 {
			log.Fatal(fmt.Errorf("failed to find wsl2-ssh-agent"))
		}

		log.Printf("kill wsl2-ssh-agent (pid: %d)", serverPid)
		stopService(serverPid)
		os.Exit(0)
	}

	// avoid multiple start
	if serverPid != -1 {
		log.Printf("wsl2-ssh-agent (pid: %d) is already running; exit", serverPid)
		fmt.Println(output)
		os.Exit(0)
	}

	// daemonize
	if !c.foreground {
		if parent {
			log.Printf("daemonize: start")
			args := []string{"-socket", c.socketPath}
			if c.logFile != "" {
				args = append(args, "-log", c.logFile)
			}
			startDaemonizing(args...)
		} else {
			completeDaemonizing(output)
			log.Printf("daemonize: completed")
		}
	}

	// set up signal handlers
	signal.Ignore(syscall.SIGPIPE)
	ctx, _ := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	return ctx
}

func (c *config) setupLogFile() {
	var logFile *os.File

	if c.logFile != "" {
		f, err := os.OpenFile(c.logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			log.Fatal(fmt.Errorf("failed to open a log file: %s", err))
		}
		logFile = f
	}

	if logFile != nil {
		if c.verbose {
			logOutput = io.MultiWriter(logFile, os.Stdout)
		} else {
			logOutput = logFile
		}
	} else {
		if c.verbose {
			logOutput = os.Stdout
		} else {
			logOutput = io.Discard
		}
	}
	log.SetOutput(logOutput)
	log.SetPrefix("[L] ")
}

// find the existing server by getsockopt
func findRunningServerPid(path string) int {
	// try to connect to the existing server via UNIX domain socket
	conn, err := net.Dial("unix", path)

	if err != nil {
		// failed to connect
		if errors.Is(err, syscall.ENOENT) {
			// no UNIX domain socket; cannot find the server
			return -1
		}
		if errors.Is(err, syscall.ECONNREFUSED) {
			// presumably the existing server has already aborted;
			// remove the UNIX domain socket
			err = os.Remove(path)
			if err != nil {
				log.Fatal(fmt.Errorf("failed to remove %s: %s", path, err))
			}
			return -1
		}
		log.Fatal(fmt.Errorf("failed to connect to %s: %s", path, err))
	}

	// connected
	defer conn.Close()

	// identify the pid of the existing server
	file, err := conn.(*net.UnixConn).File()
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	cred, err := syscall.GetsockoptUcred(int(file.Fd()), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	if err != nil {
		log.Fatal(err)
	}

	return int(cred.Pid)
}

// stop the running server
func stopService(pid int) {
	process, err := os.FindProcess(pid)
	if err != nil {
		log.Fatal(err)
	}
	err = process.Signal(syscall.SIGTERM)
	if err != nil {
		log.Fatal(err)
	}
	_, err = process.Wait()
	if err != nil {
		log.Fatal(err)
	}
}
