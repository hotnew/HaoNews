package haonews

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

type libp2pListenSpec struct {
	index     int
	hostProto string
	host      string
	transport string
	port      int
	parts     []string
}

func resolveBitTorrentListenAddr(addr string) (string, error) {
	addr = normalizeBitTorrentListen(addr)
	if strings.TrimSpace(addr) == "" {
		return "", nil
	}
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, nil
	}
	port, err := strconv.Atoi(strings.TrimSpace(portText))
	if err != nil || port <= 0 {
		return addr, nil
	}
	for candidate := port; candidate <= 65535; candidate++ {
		bind := net.JoinHostPort(host, strconv.Itoa(candidate))
		tcpListener, err := net.Listen("tcp", bind)
		if err != nil {
			if isAddrInUse(err) {
				continue
			}
			return "", err
		}
		udpConn, err := net.ListenPacket("udp", bind)
		if err != nil {
			_ = tcpListener.Close()
			if isAddrInUse(err) {
				continue
			}
			return "", err
		}
		_ = udpConn.Close()
		_ = tcpListener.Close()
		return normalizeBitTorrentListen(bind), nil
	}
	return "", fmt.Errorf("no available bittorrent listen port found starting from %s", addr)
}

func resolveLibP2PListenAddrs(addrs []string) ([]string, error) {
	out := append([]string(nil), addrs...)
	if len(out) == 0 {
		return nil, nil
	}
	grouped := map[string][]libp2pListenSpec{}
	for idx, raw := range out {
		spec, ok := parseLibP2PListenSpec(idx, raw)
		if !ok || spec.port <= 0 {
			continue
		}
		key := spec.hostProto + "\x00" + spec.host + "\x00" + strconv.Itoa(spec.port)
		grouped[key] = append(grouped[key], spec)
	}
	for _, specs := range grouped {
		resolvedPort, err := resolveLibP2PListenPort(specs)
		if err != nil {
			return nil, err
		}
		for _, spec := range specs {
			parts := append([]string(nil), spec.parts...)
			parts[3] = strconv.Itoa(resolvedPort)
			out[spec.index] = "/" + strings.Join(parts, "/")
		}
	}
	return out, nil
}

func ResolveLibP2PListenAddrs(addrs []string) ([]string, error) {
	return resolveLibP2PListenAddrs(addrs)
}

func parseLibP2PListenSpec(index int, raw string) (libp2pListenSpec, bool) {
	trimmed := strings.Trim(strings.TrimSpace(raw), "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 4 {
		return libp2pListenSpec{}, false
	}
	transport := strings.ToLower(strings.TrimSpace(parts[2]))
	if transport != "tcp" && transport != "udp" {
		return libp2pListenSpec{}, false
	}
	port, err := strconv.Atoi(strings.TrimSpace(parts[3]))
	if err != nil {
		return libp2pListenSpec{}, false
	}
	return libp2pListenSpec{
		index:     index,
		hostProto: strings.ToLower(strings.TrimSpace(parts[0])),
		host:      strings.TrimSpace(parts[1]),
		transport: transport,
		port:      port,
		parts:     parts,
	}, true
}

func resolveLibP2PListenPort(specs []libp2pListenSpec) (int, error) {
	if len(specs) == 0 {
		return 0, nil
	}
	base := specs[0].port
	for candidate := base; candidate <= 65535; candidate++ {
		closers := make([]func() error, 0, len(specs))
		ok := true
		for _, spec := range specs {
			bind := net.JoinHostPort(spec.host, strconv.Itoa(candidate))
			switch spec.transport {
			case "tcp":
				listener, err := net.Listen("tcp", bind)
				if err != nil {
					if isAddrInUse(err) {
						ok = false
						break
					}
					return 0, err
				}
				closers = append(closers, listener.Close)
			case "udp":
				conn, err := net.ListenPacket("udp", bind)
				if err != nil {
					if isAddrInUse(err) {
						ok = false
						break
					}
					return 0, err
				}
				closers = append(closers, conn.Close)
			}
		}
		for _, closeFn := range closers {
			_ = closeFn()
		}
		if ok {
			return candidate, nil
		}
	}
	return 0, fmt.Errorf("no available libp2p listen port found starting from %d", base)
}

func isAddrInUse(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "address already in use")
}
