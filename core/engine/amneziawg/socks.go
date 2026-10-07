package amneziawg

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"
)

type dialFunc func(network, address string) (net.Conn, error)

const (
	socksVersion      = 5
	socksCmdConnect   = 1
	socksCmdAssociate = 3
	socksATYPIPv4     = 1
	socksATYPDomain   = 3
	socksATYPIPv6     = 4
)

type socksServer struct {
	listener net.Listener
	dial     dialFunc
	mu       sync.Mutex
	closed   bool
}

func serveSOCKS5(listener net.Listener, dial dialFunc) *socksServer {
	server := &socksServer{listener: listener, dial: dial}
	go server.loop()
	return server
}

func (s *socksServer) Addr() string {
	return s.listener.Addr().String()
}

func (s *socksServer) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return s.listener.Close()
}

func (s *socksServer) loop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *socksServer) handle(conn net.Conn) {
	defer conn.Close()
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return
	}
	if header[0] != socksVersion {
		return
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return
	}
	if _, err := conn.Write([]byte{socksVersion, 0}); err != nil {
		return
	}
	req := make([]byte, 4)
	if _, err := io.ReadFull(conn, req); err != nil {
		return
	}
	if req[0] != socksVersion {
		return
	}
	switch req[1] {
	case socksCmdConnect:
		s.handleConnect(conn, req[3])
	case socksCmdAssociate:
		s.handleAssociate(conn, req[3])
	default:
		_, _ = conn.Write([]byte{socksVersion, 7, 0, 1, 0, 0, 0, 0, 0, 0})
	}
}

func (s *socksServer) handleConnect(conn net.Conn, atyp byte) {
	host, err := readSOCKSAddr(conn, atyp)
	if err != nil {
		_, _ = conn.Write([]byte{socksVersion, 1, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return
	}
	port := binary.BigEndian.Uint16(portBuf)
	target := net.JoinHostPort(host, strconv.Itoa(int(port)))
	remote, err := s.dial("tcp", target)
	if err != nil {
		_, _ = conn.Write([]byte{socksVersion, 5, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	defer remote.Close()
	if _, err := conn.Write([]byte{socksVersion, 0, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}
	errc := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(remote, conn)
		errc <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(conn, remote)
		errc <- struct{}{}
	}()
	<-errc
}

func (s *socksServer) handleAssociate(conn net.Conn, atyp byte) {
	if _, err := readSOCKSAddr(conn, atyp); err != nil {
		_, _ = conn.Write([]byte{socksVersion, 1, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	if _, err := io.ReadFull(conn, make([]byte, 2)); err != nil {
		return
	}
	udpConn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		_, _ = conn.Write([]byte{socksVersion, 1, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	defer udpConn.Close()
	reply, err := socksBindReply(udpConn.LocalAddr())
	if err != nil {
		_, _ = conn.Write([]byte{socksVersion, 1, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	if _, err := conn.Write(reply); err != nil {
		return
	}
	done := make(chan struct{})
	go func() {
		s.relayUDP(udpConn)
		close(done)
	}()
	_, _ = io.Copy(io.Discard, conn)
	_ = udpConn.Close()
	<-done
}

func (s *socksServer) relayUDP(udpConn net.PacketConn) {
	buf := make([]byte, 65535)
	for {
		n, clientAddr, err := udpConn.ReadFrom(buf)
		if err != nil {
			return
		}
		host, port, payload, ok := parseSOCKSUDP(buf[:n])
		if !ok || len(payload) == 0 {
			continue
		}
		target := net.JoinHostPort(host, strconv.Itoa(port))
		remote, err := s.dial("udp", target)
		if err != nil {
			continue
		}
		_ = remote.SetDeadline(time.Now().Add(5 * time.Second))
		if _, err := remote.Write(payload); err != nil {
			_ = remote.Close()
			continue
		}
		resp := make([]byte, 65535)
		rn, err := remote.Read(resp)
		_ = remote.Close()
		if err != nil || rn == 0 {
			continue
		}
		packet := encodeSOCKSUDP(host, port, resp[:rn])
		_, _ = udpConn.WriteTo(packet, clientAddr)
	}
}

func socksBindReply(addr net.Addr) ([]byte, error) {
	udpAddr, ok := addr.(*net.UDPAddr)
	if !ok || udpAddr == nil {
		return nil, fmt.Errorf("amneziawg socks udp bind address is invalid")
	}
	ip := udpAddr.IP.To4()
	if ip == nil {
		ip = net.IPv4(127, 0, 0, 1)
	}
	reply := []byte{socksVersion, 0, 0, socksATYPIPv4, ip[0], ip[1], ip[2], ip[3], 0, 0}
	binary.BigEndian.PutUint16(reply[8:], uint16(udpAddr.Port))
	return reply, nil
}

func parseSOCKSUDP(packet []byte) (host string, port int, payload []byte, ok bool) {
	if len(packet) < 7 {
		return "", 0, nil, false
	}
	if packet[0] != 0 || packet[1] != 0 {
		return "", 0, nil, false
	}
	if packet[2] != 0 {
		return "", 0, nil, false
	}
	atyp := packet[3]
	rest := packet[4:]
	switch atyp {
	case socksATYPIPv4:
		if len(rest) < 6 {
			return "", 0, nil, false
		}
		host = net.IP(rest[:4]).String()
		port = int(binary.BigEndian.Uint16(rest[4:6]))
		return host, port, rest[6:], true
	case socksATYPIPv6:
		if len(rest) < 18 {
			return "", 0, nil, false
		}
		host = net.IP(rest[:16]).String()
		port = int(binary.BigEndian.Uint16(rest[16:18]))
		return host, port, rest[18:], true
	case socksATYPDomain:
		if len(rest) < 1 {
			return "", 0, nil, false
		}
		size := int(rest[0])
		if len(rest) < 1+size+2 {
			return "", 0, nil, false
		}
		host = string(rest[1 : 1+size])
		port = int(binary.BigEndian.Uint16(rest[1+size : 1+size+2]))
		return host, port, rest[1+size+2:], true
	default:
		return "", 0, nil, false
	}
}

func encodeSOCKSUDP(host string, port int, payload []byte) []byte {
	ip := net.ParseIP(host)
	if v4 := ip.To4(); v4 != nil {
		packet := make([]byte, 10+len(payload))
		packet[3] = socksATYPIPv4
		copy(packet[4:8], v4)
		binary.BigEndian.PutUint16(packet[8:10], uint16(port))
		copy(packet[10:], payload)
		return packet
	}
	if ip != nil && ip.To16() != nil {
		packet := make([]byte, 22+len(payload))
		packet[3] = socksATYPIPv6
		copy(packet[4:20], ip.To16())
		binary.BigEndian.PutUint16(packet[20:22], uint16(port))
		copy(packet[22:], payload)
		return packet
	}
	hostBytes := []byte(host)
	packet := make([]byte, 7+len(hostBytes)+len(payload))
	packet[3] = socksATYPDomain
	packet[4] = byte(len(hostBytes))
	copy(packet[5:], hostBytes)
	binary.BigEndian.PutUint16(packet[5+len(hostBytes):], uint16(port))
	copy(packet[7+len(hostBytes):], payload)
	return packet
}

func readSOCKSAddr(r io.Reader, atyp byte) (string, error) {
	switch atyp {
	case socksATYPIPv4:
		addr := make([]byte, 4)
		if _, err := io.ReadFull(r, addr); err != nil {
			return "", err
		}
		return net.IP(addr).String(), nil
	case socksATYPIPv6:
		addr := make([]byte, 16)
		if _, err := io.ReadFull(r, addr); err != nil {
			return "", err
		}
		return net.IP(addr).String(), nil
	case socksATYPDomain:
		var size [1]byte
		if _, err := io.ReadFull(r, size[:]); err != nil {
			return "", err
		}
		host := make([]byte, size[0])
		if _, err := io.ReadFull(r, host); err != nil {
			return "", err
		}
		return string(host), nil
	default:
		return "", fmt.Errorf("unsupported socks address type")
	}
}
