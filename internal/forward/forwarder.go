package forward

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

func StartTCPForwarder(ctx context.Context, listenAddress string, targetAddress string) (string, func(), error) {
	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		return "", nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go proxyTCPConnection(ctx, conn, targetAddress)
		}
	}()

	stop := func() {
		cancel()
		_ = listener.Close()
		<-done
	}
	return listener.Addr().String(), stop, nil
}

func proxyTCPConnection(ctx context.Context, downstream net.Conn, targetAddress string) {
	defer func() { _ = downstream.Close() }()

	upstream, err := net.DialTimeout("tcp", targetAddress, time.Second)
	if err != nil {
		return
	}
	defer func() { _ = upstream.Close() }()

	var once sync.Once
	closeBoth := func() {
		_ = downstream.Close()
		_ = upstream.Close()
	}
	connectionDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			once.Do(closeBoth)
		case <-connectionDone:
		}
	}()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(upstream, downstream)
		once.Do(closeBoth)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(downstream, upstream)
		once.Do(closeBoth)
	}()
	wg.Wait()
	close(connectionDone)
}

func StartUDPForwarder(ctx context.Context, listenAddress string, targetAddress string) (string, func(), error) {
	listener, err := net.ListenPacket("udp", listenAddress)
	if err != nil {
		return "", nil, err
	}
	target, err := net.ResolveUDPAddr("udp", targetAddress)
	if err != nil {
		_ = listener.Close()
		return "", nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		buffer := make([]byte, 65535)
		for {
			n, clientAddress, err := listener.ReadFrom(buffer)
			if err != nil {
				return
			}
			payload := append([]byte(nil), buffer[:n]...)
			go proxyUDPDatagram(ctx, listener, clientAddress, target, payload)
		}
	}()

	stop := func() {
		cancel()
		_ = listener.Close()
		<-done
	}
	return listener.LocalAddr().String(), stop, nil
}

func proxyUDPDatagram(ctx context.Context, listener net.PacketConn, clientAddress net.Addr, targetAddress *net.UDPAddr, payload []byte) {
	upstream, err := net.DialUDP("udp", nil, targetAddress)
	if err != nil {
		return
	}
	defer func() { _ = upstream.Close() }()

	if _, err := upstream.Write(payload); err != nil {
		return
	}
	if err := upstream.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		return
	}
	buffer := make([]byte, 65535)
	n, err := upstream.Read(buffer)
	if err != nil {
		if errors.Is(err, net.ErrClosed) {
			return
		}
		return
	}
	select {
	case <-ctx.Done():
		return
	default:
		_, _ = listener.WriteTo(buffer[:n], clientAddress)
	}
}
