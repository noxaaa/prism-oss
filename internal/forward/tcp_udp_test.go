package forward

import (
	"context"
	"io"
	"net"
	"runtime"
	"testing"
	"time"
)

func TestTCPForwarderCopiesBytesBothDirections(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	targetAddr, closeTarget := startTCPEchoServer(t)
	defer closeTarget()

	listenAddr, stopForwarder, err := StartTCPForwarder(ctx, "127.0.0.1:0", targetAddr)
	if err != nil {
		t.Fatalf("start forwarder: %v", err)
	}
	defer stopForwarder()

	conn, err := net.DialTimeout("tcp", listenAddr, time.Second)
	if err != nil {
		t.Fatalf("dial forwarder: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatalf("write to forwarder: %v", err)
	}
	buffer := make([]byte, 5)
	if _, err := io.ReadFull(conn, buffer); err != nil {
		t.Fatalf("read echoed bytes: %v", err)
	}
	if string(buffer) != "hello" {
		t.Fatalf("expected echo, got %q", buffer)
	}
}

func TestTCPProxyReleasesContextWatcherWhenConnectionEnds(t *testing.T) {
	targetAddr, closeTarget := startTCPClosingServer(t)
	defer closeTarget()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	baseline := runtime.NumGoroutine()
	const connections = 20
	for range connections {
		client, server := net.Pipe()
		done := make(chan struct{})
		go func() {
			defer close(done)
			proxyTCPConnection(ctx, server, targetAddr)
		}()
		_ = client.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("proxy connection did not finish")
		}
	}

	time.Sleep(50 * time.Millisecond)
	if leaked := runtime.NumGoroutine() - baseline; leaked > connections/2 {
		t.Fatalf("connection cleanup left too many goroutines: baseline=%d leaked=%d", baseline, leaked)
	}
}

func TestUDPForwarderCopiesDatagrams(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	targetAddr, closeTarget := startUDPEchoServer(t)
	defer closeTarget()

	listenAddr, stopForwarder, err := StartUDPForwarder(ctx, "127.0.0.1:0", targetAddr)
	if err != nil {
		t.Fatalf("start udp forwarder: %v", err)
	}
	defer stopForwarder()

	conn, err := net.Dial("udp", listenAddr)
	if err != nil {
		t.Fatalf("dial udp forwarder: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatalf("write datagram: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	buffer := make([]byte, 5)
	if _, err := io.ReadFull(conn, buffer); err != nil {
		t.Fatalf("read datagram: %v", err)
	}
	if string(buffer) != "hello" {
		t.Fatalf("expected echo, got %q", buffer)
	}
}

func startTCPEchoServer(t *testing.T) (string, func()) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp echo: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()

	return listener.Addr().String(), func() {
		_ = listener.Close()
		<-done
	}
}

func startTCPClosingServer(t *testing.T) (string, func()) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp close server: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	return listener.Addr().String(), func() {
		_ = listener.Close()
		<-done
	}
}

func startUDPEchoServer(t *testing.T) (string, func()) {
	t.Helper()

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp echo: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		buffer := make([]byte, 2048)
		for {
			n, addr, err := conn.ReadFrom(buffer)
			if err != nil {
				return
			}
			_, _ = conn.WriteTo(buffer[:n], addr)
		}
	}()

	return conn.LocalAddr().String(), func() {
		_ = conn.Close()
		<-done
	}
}
