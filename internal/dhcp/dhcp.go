package dhcp

import "time"

// Lease represents a DHCP lease
type Lease struct {
	IP       string    `json:"ip"`
	MAC      string    `json:"mac"`
	Hostname string    `json:"hostname"`
	Expires  time.Time `json:"expires"`
}

func Init() {
	// TODO: start dnsmasq, generate config
}

// GetLeases returns current DHCP leases from dnsmasq lease file
func GetLeases() []Lease {
	// TODO: parse /var/lib/misc/dnsmasq.leases or custom path
	return []Lease{}
}

// Reload regenerates dnsmasq config and sends SIGHUP
func Reload() {
	// TODO: implement
}
