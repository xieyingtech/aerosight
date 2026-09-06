package main

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestHTTPServerConnectionDeadlines(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := newHTTPServer(ctx, "127.0.0.1:0", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	if s.ReadHeaderTimeout != 5*time.Second || s.IdleTimeout != 60*time.Second || s.WriteTimeout != 0 || s.ReadTimeout != 0 || s.BaseContext(nil) != ctx {
		t.Fatalf("server policy %+v", s)
	}
	// Accelerate the same server's network deadlines; production values above are
	// checked separately so this regression test does not wait a full minute.
	s.ReadHeaderTimeout = 100 * time.Millisecond
	s.IdleTimeout = 100 * time.Millisecond
	listener, err := net.Listen("tcp", s.Addr)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Serve(listener) }()
	defer func() { s.Close(); <-done }()
	for _, idle := range []bool{false, true} {
		conn, err := net.Dial("tcp", listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		conn.SetDeadline(time.Now().Add(2 * time.Second))
		reader := bufio.NewReader(conn)
		if idle {
			io.WriteString(conn, "GET / HTTP/1.1\r\nHost: localhost\r\n\r\n")
			res, err := http.ReadResponse(reader, nil)
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
			if res.StatusCode != 204 {
				t.Fatalf("initial request %d", res.StatusCode)
			}
		} else {
			io.WriteString(conn, "GET / HTTP/1.1\r\nHost: localhost\r\n")
		}
		_, err = io.ReadAll(reader)
		conn.Close()
		if err != nil {
			t.Fatalf("connection did not close within server deadline (idle=%v): %v", idle, err)
		}
	}
}
