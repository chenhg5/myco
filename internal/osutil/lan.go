package osutil

import (
	"net"
	"os"
	"strings"
)

// LANAddr returns this machine's IPv4 address on the local network (a
// private address such as 192.168.1.20), or "" if it has none. It prefers
// the address of the default route and skips virtual interfaces (Docker,
// VPNs, VMs) when it has to scan. MYCO_LAN_ADDR overrides the choice (a
// machine on several networks).
func LANAddr() string {
	if a := strings.TrimSpace(os.Getenv("MYCO_LAN_ADDR")); a != "" {
		return a
	}
	// Connecting a UDP socket sends nothing; it only picks the source address.
	if c, err := net.Dial("udp4", "192.0.2.1:9"); err == nil {
		ip := c.LocalAddr().(*net.UDPAddr).IP
		c.Close()
		if ip.IsPrivate() {
			return ip.String()
		}
	}
	ifs, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 || virtualInterface(i.Name) {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && n.IP.IsPrivate() {
				return n.IP.String()
			}
		}
	}
	return ""
}

func virtualInterface(name string) bool {
	for _, p := range []string{"docker", "br-", "veth", "virbr", "vmnet", "vboxnet", "utun", "tun", "tap", "wg", "tailscale", "zt", "bridge", "awdl", "llw", "vEthernet"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}
