package desk

import (
	"errors"
	"net"
)

type NetworkAddress struct {
	Interface string `json:"interface"`
	Address   string `json:"address"`
}

// DetectPublicNIC asks the OS to choose a source address for an external route.
// UDP connect selects a route without sending any application packet. The
// resulting NIC address may be private behind NAT; no external IP service is used.
func DetectPublicNIC() (NetworkAddress, error) {
	for _, target := range []string{"1.1.1.1:80", "[2606:4700:4700::1111]:80"} {
		raddr, err := net.ResolveUDPAddr("udp", target)
		if err != nil {
			continue
		}
		conn, err := net.DialUDP("udp", nil, raddr)
		if err != nil {
			continue
		}
		ip := conn.LocalAddr().(*net.UDPAddr).IP
		conn.Close()
		if !ip.IsGlobalUnicast() || ip.IsLoopback() {
			continue
		}
		interfaces, err := net.Interfaces()
		if err != nil {
			return NetworkAddress{}, errors.New("cannot enumerate network interfaces")
		}
		for _, iface := range interfaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				candidate, _, err := net.ParseCIDR(addr.String())
				if err == nil && candidate.Equal(ip) {
					return NetworkAddress{Interface: iface.Name, Address: ip.String()}, nil
				}
			}
		}
	}
	return NetworkAddress{}, errors.New("cannot detect an active default-route interface; set --listen and --public-url explicitly")
}

// ResolveNetwork keeps the ordinary service loopback-only. Enabling Telegram
// explicitly opts into binding the detected interface unless listen is supplied.
func ResolveNetwork(listen, publicURL string, listenExplicit, telegram bool, detect func() (NetworkAddress, error)) (string, string, error) {
	if publicURL != "" {
		if err := ValidateURL(publicURL); err != nil {
			return "", "", err
		}
		return listen, "", nil
	}
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "", "", err
	}
	if telegram && !listenExplicit {
		nic, err := detect()
		if err != nil {
			return "", "", err
		}
		return net.JoinHostPort(nic.Address, port), nic.Address, nil
	}
	ip := net.ParseIP(host)
	if host == "" || (ip != nil && ip.IsUnspecified()) {
		nic, err := detect()
		if err != nil {
			return "", "", err
		}
		if ip != nil && ip.To4() != nil && net.ParseIP(nic.Address).To4() == nil {
			return "", "", errors.New("IPv4 listener cannot advertise an IPv6 interface; set --public-url")
		}
		return listen, nic.Address, nil
	}
	if telegram && (ip == nil || !ip.IsGlobalUnicast() || ip.IsLoopback()) {
		return "", "", errors.New("Telegram issue links need a reachable interface; omit --listen for autodetection or set --public-url")
	}
	return listen, "", nil
}
