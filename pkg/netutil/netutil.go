package netutil

import (
	"net"
	"os"
	"path/filepath"
	"strings"
)

// InterfaceInfo holds details about a network interface
type InterfaceInfo struct {
	Name  string  `json:"name"`
	State string  `json:"state"` // "up" or "down"
	IP    *string `json:"ip"`
	MAC   string  `json:"mac"`
	Type  string  `json:"type"` // "physical", "bridge", "virtual"
}

// ListInterfaces returns all non-loopback network interfaces
func ListInterfaces() ([]InterfaceInfo, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var result []InterfaceInfo
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		// skip tunnel/vpn interfaces created by us
		if strings.HasPrefix(iface.Name, "awg") || strings.HasPrefix(iface.Name, "tun-jh") {
			continue
		}

		info := InterfaceInfo{
			Name: iface.Name,
			MAC:  iface.HardwareAddr.String(),
			Type: guessType(iface.Name),
		}

		if iface.Flags&net.FlagUp != 0 {
			info.State = "up"
		} else {
			info.State = "down"
		}

		if addrs, err := iface.Addrs(); err == nil {
			for _, addr := range addrs {
				s := addr.String()
				// skip ipv6
				if strings.Contains(s, ":") {
					continue
				}
				info.IP = &s
				break
			}
		}

		result = append(result, info)
	}
	return result, nil
}

func guessType(name string) string {
	if strings.HasPrefix(name, "br-") || strings.HasPrefix(name, "bridge") {
		return "bridge"
	}
	if strings.HasPrefix(name, "veth") || strings.HasPrefix(name, "docker") {
		return "virtual"
	}
	// check if physical by looking at /sys/class/net/<name>/device
	if _, err := os.Stat(filepath.Join("/sys/class/net", name, "device")); err == nil {
		return "physical"
	}
	return "virtual"
}

// HasInternet checks if the given interface has a default route
func HasInternet(iface string) bool {
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		// default route: destination = 00000000
		if fields[0] == iface && fields[1] == "00000000" {
			return true
		}
	}
	return false
}
