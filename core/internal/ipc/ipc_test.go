package ipc

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	msgs := [][]byte{[]byte("hello"), {}, []byte(`{"verdict":"deny"}`)}
	for _, m := range msgs {
		if err := WriteFrame(&buf, m); err != nil {
			t.Fatalf("WriteFrame: %v", err)
		}
	}
	for i, want := range msgs {
		got, err := ReadFrame(&buf)
		if err != nil {
			t.Fatalf("ReadFrame %d: %v", i, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("frame %d = %q, want %q", i, got, want)
		}
	}
}

func TestWriteFrameRejectsOversize(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteFrame(&buf, make([]byte, MaxFrame+1)); err == nil {
		t.Fatal("expected error writing an oversized frame")
	}
}

func TestControlPlaneRoundTrip(t *testing.T) {
	t.Setenv("AFTCAST_IPC_ID", "ipc-controlplane-test")
	ln, err := Listen()
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		b, err := ReadFrame(conn)
		if err != nil {
			return
		}
		_ = WriteFrame(conn, append([]byte("echo:"), b...))
	}()

	conn, err := Dial(2 * time.Second)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	if err := WriteFrame(conn, []byte("ping")); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	resp, err := ReadFrame(conn)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if string(resp) != "echo:ping" {
		t.Fatalf("response = %q, want %q", resp, "echo:ping")
	}
	wg.Wait()
}

func TestHTTPListenLocalhostAndFallback(t *testing.T) {
	ln1, port1, err := HTTPListen(0)
	if err != nil {
		t.Fatalf("HTTPListen: %v", err)
	}
	defer ln1.Close()
	if !strings.HasPrefix(ln1.Addr().String(), "127.0.0.1:") {
		t.Fatalf("listener bound to %s, want 127.0.0.1", ln1.Addr())
	}

	// Requesting the now-occupied port must fall back to a different one.
	ln2, port2, err := HTTPListen(port1)
	if err != nil {
		t.Fatalf("HTTPListen fallback: %v", err)
	}
	defer ln2.Close()
	if port2 == port1 {
		t.Fatalf("expected fallback to a different port, both %d", port1)
	}

	// The listener is a working localhost HTTP server.
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})}
	go func() { _ = srv.Serve(ln1) }()
	defer srv.Close()

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port1))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Fatalf("body = %q, want ok", body)
	}
}
