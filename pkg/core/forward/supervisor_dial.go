package forward

import (
	"net"
	"strings"
	"time"
)

func dialTCPUpstream(sendIP string, address string) (net.Conn, error) {
	dialer := net.Dialer{Timeout: time.Second}
	if localIP := net.ParseIP(strings.TrimSpace(sendIP)); localIP != nil {
		dialer.LocalAddr = &net.TCPAddr{IP: localIP}
	}
	return dialer.Dial("tcp", address)
}

func dialUDPUpstream(sendIP string, upstreamAddress *net.UDPAddr) (*net.UDPConn, error) {
	var localAddress *net.UDPAddr
	if localIP := net.ParseIP(strings.TrimSpace(sendIP)); localIP != nil {
		localAddress = &net.UDPAddr{IP: localIP}
	}
	return net.DialUDP("udp", localAddress, upstreamAddress)
}
