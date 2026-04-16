package newsplugin

import (
	"fmt"
	"net"
	"strconv"
)

const minimumDynamicPort = 10240

func pickFreeTCPPort() (int, error) {
	for i := 0; i < 64; i++ {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return 0, err
		}
		addr, ok := ln.Addr().(*net.TCPAddr)
		_ = ln.Close()
		if !ok {
			return 0, fmt.Errorf("unexpected tcp address type %T", ln.Addr())
		}
		if addr.Port >= minimumDynamicPort {
			return addr.Port, nil
		}
	}
	return 0, fmt.Errorf("failed to find a free tcp port >= %d", minimumDynamicPort)
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
