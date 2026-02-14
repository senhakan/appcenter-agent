//go:build windows

package ipc

import (
	"encoding/json"
	"net"
	"sync"
	"time"

	"github.com/Microsoft/go-winio"
)

type pipeServer struct {
	listener net.Listener
	closeCh  chan struct{}
	once     sync.Once
}

func StartPipeServer(handler Handler) (Server, error) {
	config := &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;WD)",
		MessageMode:        true,
		InputBufferSize:    65536,
		OutputBufferSize:   65536,
	}

	listener, err := winio.ListenPipe(PipeName, config)
	if err != nil {
		return nil, err
	}

	s := &pipeServer{listener: listener, closeCh: make(chan struct{})}
	go s.acceptLoop(handler)
	return s, nil
}

func (s *pipeServer) acceptLoop(handler Handler) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.closeCh:
				return
			default:
			}
			continue
		}
		go handleConnection(conn, handler)
	}
}

func (s *pipeServer) Close() error {
	var err error
	s.once.Do(func() {
		close(s.closeCh)
		err = s.listener.Close()
	})
	return err
}

func SendRequest(req Request) (*Response, error) {
	timeout := 5 * time.Second
	conn, err := winio.DialPipe(PipeName, &timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	if err := enc.Encode(req); err != nil {
		return nil, err
	}

	var resp Response
	if err := dec.Decode(&resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func handleConnection(conn net.Conn, handler Handler) {
	defer conn.Close()

	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)

	var req Request
	if err := dec.Decode(&req); err != nil {
		_ = enc.Encode(Response{Status: "error", Message: "invalid request"})
		return
	}

	resp := handler(req)
	if resp.Status == "" {
		resp.Status = "ok"
	}
	_ = enc.Encode(resp)
}
