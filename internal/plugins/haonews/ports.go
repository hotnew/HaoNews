package newsplugin

import (
	"fmt"
	"net"
	"strconv"
)

func pickFreeTCPPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected tcp address type %T", ln.Addr())
	}
	return addr.Port, nil
}

func pickFreeTCPAndUDPPort() (int, error) {
	for i := 0; i < 32; i++ {
		port, err := pickFreeTCPPort()
		if err != nil {
			return 0, err
		}
		udpAddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err != nil {
			return 0, err
		}
		conn, err := net.ListenUDP("udp", udpAddr)
		if err != nil {
			continue
		}
		_ = conn.Close()
		return port, nil
	}
	return 0, fmt.Errorf("failed to find a free tcp/udp port")
}
