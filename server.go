package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

type server struct {
	listener                net.Listener
	powershellPath string
}

func newServer(socketPath string, powershellPath string) *server {
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("start listening on %s", socketPath)

	return &server{listener, powershellPath}
}

type request struct {
	data          []byte
	resultChannel chan response
}
type response []byte

func (s *server) run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)

	go func() {
		<-ctx.Done()
		log.Println("shutdown")
		s.listener.Close()
	}()

	// invoke gorountine for ssh-agent.exe
	done := make(chan struct{}, 1)
	requestQueue := make(chan request)
	go func() {
		s.server(ctx, cancel, requestQueue, done)
	}()

	wg := &sync.WaitGroup{}
	for {
		// wait for connection
		sshClient, err := s.listener.Accept()
		if err != nil {
			log.Printf("failed to accept: %s", err)
			cancel()
			break
		}

		// invoke goroutine for ssh client
		log.Printf("ssh: connected")
		wg.Add(1)
		go s.client(wg, ctx, sshClient, requestQueue)
	}

	// wait for all ssh clients to disconnect
	wg.Wait()

	// wait for ssh-agent.exe to exit
	close(requestQueue)
	<-done
}

func (s *server) server(ctx context.Context, cancel func(), requestQueue chan request, done chan struct{}) {
	defer close(done)

	var pendingRequest *request
	retryCount := 0

	defer func() {
		// abort all pending requests
		cancel()
		if pendingRequest != nil {
			close(pendingRequest.resultChannel)
		}
		for req := range requestQueue {
			close(req.resultChannel)
		}
	}()

	for {
		// invoke PowerShell.exe
		rep, err := newRepeater(ctx, s.powershellPath)
		if err != nil {
			return
		}
		defer rep.terminate()

		// process a pending request if any
		if pendingRequest != nil && handleRequest(rep, pendingRequest) != nil {
			// fail
			retryCount += 1
			log.Printf("failed to process request (%d/3)", retryCount)
			if retryCount == 3 {
				log.Printf("give up")
				break
			}
		} else {
			// process requests sequentially
			for req := range requestQueue {
				retryCount = 0
				pendingRequest = &req
				if handleRequest(rep, pendingRequest) != nil {
					break
				}
			}
		}

		select {
		case <-ctx.Done():
			log.Printf("[W] terminated")
			return
		default:
			log.Printf("[W] terminated; retry")
		}
	}
}

type deadliner interface {
	SetReadDeadline(time.Time) error
}

var readTimeLimit = 10 * time.Second

func handleRequest(rep *repeater, req *request) error {
	_, err := rep.in.Write(req.data)
	if err != nil {
		log.Printf("failed to write to [W]: %s", err)
		return err
	}
	log.Printf("[L] -> [W] (%d B)", len(req.data))

	if out, ok := rep.out.(deadliner); ok {
		err = out.SetReadDeadline(time.Now().Add(readTimeLimit))
		if err != nil {
			log.Printf("failed to set timeout: %s", err)
		}
	}

	resp, err := readMessage(rep.out)
	if err != nil {
		log.Printf("failed to read from [W]: %s", err)
		return err
	}
	log.Printf("[L] <- [W] (%d B)", len(resp))

	if out, ok := rep.out.(deadliner); ok {
		err = out.SetReadDeadline(time.Time{})
		if err != nil {
			log.Printf("failed to set timeout: %s", err)
		}
	}

	req.resultChannel <- resp

	return nil
}

func (s *server) client(wg *sync.WaitGroup, ctx context.Context, sshClient net.Conn, requestQueue chan request) {
	defer wg.Done()
	defer sshClient.Close()

	resChan := make(chan response)

	for {
		req, err := readMessage(sshClient)
		if err != nil {
			break
		}
		log.Printf("ssh -> [L] (%d B)", len(req))

		requestQueue <- request{data: req, resultChannel: resChan}
		resp, ok := <-resChan
		if !ok {
			log.Printf("failed to get result")
			break
		}
		_, err = sshClient.Write(resp)
		if err != nil {
			log.Printf("failed to write to ssh: %s", err)
			break
		}
		log.Printf("ssh <- [L] (%d B)", len(resp))
	}
	log.Printf("ssh: closed")
}

func readMessage(from io.Reader) ([]byte, error) {
	// In ssh-agent protocol, any message consists of:
	//
	//    uint32                   message length (network order)
	//    byte                     message type
	//    byte[message length - 1] message contents
	//
	// ref: https://tools.ietf.org/html/draft-miller-ssh-agent-04

	header := make([]byte, 4)
	_, err := io.ReadFull(from, header)
	if err != nil {
		return nil, err
	}

	var len uint32
	err = binary.Read(bytes.NewReader(header), binary.BigEndian, &len)
	if err != nil {
		log.Fatal("unreachable")
	}

	body := make([]byte, len)
	_, err = io.ReadFull(from, body)
	if err != nil {
		return nil, err
	}

	return append(header, body...), nil
}
