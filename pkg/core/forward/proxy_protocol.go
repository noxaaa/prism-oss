package forward

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

type ProxyProtocolVersion int

const (
	ProxyProtocolNone ProxyProtocolVersion = iota
	ProxyProtocolV1
	ProxyProtocolV2
)

var proxyV2Signature = []byte{0x0d, 0x0a, 0x0d, 0x0a, 0x00, 0x0d, 0x0a, 0x51, 0x55, 0x49, 0x54, 0x0a}

type ProxyInfo struct {
	Version         ProxyProtocolVersion
	SourceIP        net.IP
	DestinationIP   net.IP
	SourcePort      int
	DestinationPort int
}

func ParseProxyHeader(data []byte) (ProxyInfo, []byte, error) {
	if bytes.HasPrefix(data, []byte("PROXY ")) {
		return parseProxyV1(data)
	}
	if bytes.HasPrefix(data, proxyV2Signature) {
		return parseProxyV2(data)
	}
	return ProxyInfo{Version: ProxyProtocolNone}, data, nil
}

func BuildProxyHeader(info ProxyInfo) ([]byte, error) {
	switch info.Version {
	case ProxyProtocolV1:
		return buildProxyV1(info)
	case ProxyProtocolV2:
		return buildProxyV2(info)
	default:
		return nil, errors.New("unsupported proxy protocol version")
	}
}

func parseProxyV1(data []byte) (ProxyInfo, []byte, error) {
	index := bytes.Index(data, []byte("\r\n"))
	if index < 0 {
		return ProxyInfo{}, nil, errors.New("proxy v1 header missing terminator")
	}
	fields := strings.Fields(string(data[:index]))
	if len(fields) != 6 || fields[0] != "PROXY" || (fields[1] != "TCP4" && fields[1] != "TCP6") {
		return ProxyInfo{}, nil, errors.New("unsupported proxy v1 header")
	}

	sourcePort, err := parseProxyV1Port(fields[4], "source")
	if err != nil {
		return ProxyInfo{}, nil, err
	}
	destinationPort, err := parseProxyV1Port(fields[5], "destination")
	if err != nil {
		return ProxyInfo{}, nil, err
	}
	sourceIP, err := parseProxyV1Address(fields[1], fields[2], "source")
	if err != nil {
		return ProxyInfo{}, nil, err
	}
	destinationIP, err := parseProxyV1Address(fields[1], fields[3], "destination")
	if err != nil {
		return ProxyInfo{}, nil, err
	}

	return ProxyInfo{
		Version:         ProxyProtocolV1,
		SourceIP:        sourceIP,
		DestinationIP:   destinationIP,
		SourcePort:      sourcePort,
		DestinationPort: destinationPort,
	}, data[index+2:], nil
}

func parseProxyV1Address(protocol string, value string, label string) (net.IP, error) {
	ip := net.ParseIP(value)
	if ip == nil {
		return nil, fmt.Errorf("proxy v1 %s address must be valid %s", label, protocol)
	}
	if protocol == "TCP4" {
		ip = ip.To4()
		if ip == nil {
			return nil, fmt.Errorf("proxy v1 %s address must be valid TCP4", label)
		}
		return ip, nil
	}
	if ip.To4() != nil || ip.To16() == nil {
		return nil, fmt.Errorf("proxy v1 %s address must be valid TCP6", label)
	}
	return ip, nil
}

func parseProxyV1Port(value string, label string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s port: %w", label, err)
	}
	if port < 0 || port > 65535 {
		return 0, fmt.Errorf("proxy v1 %s port out of range", label)
	}
	return port, nil
}

func buildProxyV1(info ProxyInfo) ([]byte, error) {
	sourceIP := info.SourceIP.To4()
	destinationIP := info.DestinationIP.To4()
	if sourceIP == nil || destinationIP == nil {
		return nil, errors.New("proxy v1 builder only supports TCP4")
	}
	return []byte(fmt.Sprintf(
		"PROXY TCP4 %s %s %d %d\r\n",
		sourceIP.String(),
		destinationIP.String(),
		info.SourcePort,
		info.DestinationPort,
	)), nil
}

func parseProxyV2(data []byte) (ProxyInfo, []byte, error) {
	if len(data) < 16 {
		return ProxyInfo{}, nil, errors.New("proxy v2 header too short")
	}
	versionCommand := data[12]
	familyProtocol := data[13]
	length := int(binary.BigEndian.Uint16(data[14:16]))
	if versionCommand != 0x21 {
		return ProxyInfo{}, nil, errors.New("unsupported proxy v2 header")
	}
	if len(data) < 16+length {
		return ProxyInfo{}, nil, errors.New("proxy v2 address block truncated")
	}
	switch familyProtocol {
	case 0x11:
		if length < 12 {
			return ProxyInfo{}, nil, errors.New("proxy v2 tcp4 address block truncated")
		}
		addresses := data[16 : 16+12]
		return ProxyInfo{
			Version:         ProxyProtocolV2,
			SourceIP:        net.IPv4(addresses[0], addresses[1], addresses[2], addresses[3]),
			DestinationIP:   net.IPv4(addresses[4], addresses[5], addresses[6], addresses[7]),
			SourcePort:      int(binary.BigEndian.Uint16(addresses[8:10])),
			DestinationPort: int(binary.BigEndian.Uint16(addresses[10:12])),
		}, data[16+length:], nil
	case 0x21:
		if length < 36 {
			return ProxyInfo{}, nil, errors.New("proxy v2 tcp6 address block truncated")
		}
		addresses := data[16 : 16+36]
		return ProxyInfo{
			Version:         ProxyProtocolV2,
			SourceIP:        append(net.IP(nil), addresses[0:16]...),
			DestinationIP:   append(net.IP(nil), addresses[16:32]...),
			SourcePort:      int(binary.BigEndian.Uint16(addresses[32:34])),
			DestinationPort: int(binary.BigEndian.Uint16(addresses[34:36])),
		}, data[16+length:], nil
	default:
		return ProxyInfo{}, nil, errors.New("unsupported proxy v2 header")
	}
}

func buildProxyV2(info ProxyInfo) ([]byte, error) {
	sourceIP := info.SourceIP.To4()
	destinationIP := info.DestinationIP.To4()
	if sourceIP == nil || destinationIP == nil {
		return nil, errors.New("proxy v2 builder only supports TCP4")
	}
	header := make([]byte, 28)
	copy(header[:12], proxyV2Signature)
	header[12] = 0x21
	header[13] = 0x11
	binary.BigEndian.PutUint16(header[14:16], 12)
	copy(header[16:20], sourceIP)
	copy(header[20:24], destinationIP)
	binary.BigEndian.PutUint16(header[24:26], uint16(info.SourcePort))
	binary.BigEndian.PutUint16(header[26:28], uint16(info.DestinationPort))
	return header, nil
}
