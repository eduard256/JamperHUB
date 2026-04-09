package netutil

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

const (
	routeTable = "100"    // custom routing table for VPN traffic
	fwMark     = "0x1234" // mark for tun2socks loop prevention
)

// SetupOutputInterface assigns IP to the output interface and brings it up.
// e.g. SetupOutputInterface("enp6s19", "198.18.0.1/24")
func SetupOutputInterface(iface, cidr string) error {
	// bring up
	if err := run("ip", "link", "set", iface, "up"); err != nil {
		return fmt.Errorf("netutil: link up %s: %w", iface, err)
	}

	// check if already has this address
	out, _ := exec.Command("ip", "-4", "addr", "show", iface).CombinedOutput()
	if strings.Contains(string(out), cidr) {
		return nil // already configured
	}

	// flush old addresses
	run("ip", "addr", "flush", "dev", iface)

	// assign
	if err := run("ip", "addr", "add", cidr, "dev", iface); err != nil {
		return fmt.Errorf("netutil: addr add %s on %s: %w", cidr, iface, err)
	}

	log.Printf("[netutil] output interface %s configured with %s", iface, cidr)
	return nil
}

// EnableForwarding enables IPv4 packet forwarding
func EnableForwarding() error {
	if err := os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0644); err != nil {
		return fmt.Errorf("netutil: enable forwarding: %w", err)
	}
	log.Printf("[netutil] ip_forward=1")
	return nil
}

// SetupNAT sets up iptables MASQUERADE for traffic from client subnet
// going through VPN tunnels or direct.
// inputIface is the WAN interface (e.g. enp6s18).
// subnet is the client subnet (e.g. 198.18.0.0/24).
func SetupNAT(inputIface, subnet string) error {
	chain := "JAMPERHUB-NAT"

	// create our chain (ignore error if exists)
	run("iptables", "-t", "nat", "-N", chain)

	// flush our chain
	run("iptables", "-t", "nat", "-F", chain)

	// masquerade traffic from client subnet
	if err := run("iptables", "-t", "nat", "-A", chain,
		"-s", subnet, "-j", "MASQUERADE"); err != nil {
		return fmt.Errorf("netutil: masquerade: %w", err)
	}

	// insert jump to our chain in POSTROUTING (if not already there)
	out, _ := exec.Command("iptables", "-t", "nat", "-S", "POSTROUTING").CombinedOutput()
	if !strings.Contains(string(out), chain) {
		if err := run("iptables", "-t", "nat", "-A", "POSTROUTING", "-j", chain); err != nil {
			return fmt.Errorf("netutil: postrouting jump: %w", err)
		}
	}

	log.Printf("[netutil] NAT configured for %s", subnet)
	return nil
}

// SetupForwarding allows forwarding between output interface and VPN tunnels.
// outputIface is the LAN-facing interface (e.g. enp6s19).
func SetupForwarding(outputIface string) error {
	chain := "JAMPERHUB-FWD"

	run("iptables", "-N", chain)
	run("iptables", "-F", chain)

	// allow forwarded traffic from output interface (client devices)
	if err := run("iptables", "-A", chain,
		"-i", outputIface, "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("netutil: forward in: %w", err)
	}

	// allow return traffic
	if err := run("iptables", "-A", chain,
		"-o", outputIface, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("netutil: forward out: %w", err)
	}

	// insert jump in FORWARD (if not already)
	out, _ := exec.Command("iptables", "-S", "FORWARD").CombinedOutput()
	if !strings.Contains(string(out), chain) {
		if err := run("iptables", "-A", "FORWARD", "-j", chain); err != nil {
			return fmt.Errorf("netutil: forward jump: %w", err)
		}
	}

	log.Printf("[netutil] forwarding configured for %s", outputIface)
	return nil
}

// SetActiveRoute sets the default route for client traffic through the given tunnel interface.
// Uses policy routing (table 100) so we don't break the host's own default route.
func SetActiveRoute(tunIface string) error {
	// flush our table
	run("ip", "route", "flush", "table", routeTable)

	// add default route via tunnel
	if err := run("ip", "route", "add", "default", "dev", tunIface, "table", routeTable); err != nil {
		return fmt.Errorf("netutil: route add default dev %s: %w", tunIface, err)
	}

	// add ip rule: traffic from client subnet -> our table (if not exists)
	ensureRule("from", "198.18.0.0/24", "lookup", routeTable)

	log.Printf("[netutil] active route -> %s", tunIface)
	return nil
}

// SetDirectRoute sets fallback direct routing (no VPN, traffic goes through WAN)
func SetDirectRoute(inputIface, gateway string) error {
	run("ip", "route", "flush", "table", routeTable)

	if gateway != "" {
		if err := run("ip", "route", "add", "default", "via", gateway, "dev", inputIface, "table", routeTable); err != nil {
			return fmt.Errorf("netutil: direct route: %w", err)
		}
	} else {
		// no gateway known, just use the input interface
		if err := run("ip", "route", "add", "default", "dev", inputIface, "table", routeTable); err != nil {
			return fmt.Errorf("netutil: direct route: %w", err)
		}
	}

	ensureRule("from", "198.18.0.0/24", "lookup", routeTable)

	log.Printf("[netutil] active route -> direct (%s)", inputIface)
	return nil
}

// SetupTun2socksRouting adds fwmark-based routing to prevent tun2socks loop.
// tun2socks marks its own packets with fwmark, those bypass the VPN table
// and go directly through the WAN interface.
func SetupTun2socksRouting(inputIface string) error {
	markTable := "200"

	// create rule: packets with our mark -> table 200 (direct)
	ensureRule("fwmark", fwMark, "lookup", markTable)

	// table 200: default via WAN
	run("ip", "route", "flush", "table", markTable)

	// copy default gateway from main table
	gw := getDefaultGateway()
	if gw != "" {
		run("ip", "route", "add", "default", "via", gw, "dev", inputIface, "table", markTable)
	} else {
		run("ip", "route", "add", "default", "dev", inputIface, "table", markTable)
	}

	log.Printf("[netutil] tun2socks fwmark routing via %s (mark %s -> table %s)", inputIface, fwMark, markTable)
	return nil
}

// FwMark returns the fwmark string for tun2socks to use
func FwMark() string {
	return fwMark
}

// CleanupRouting removes all JamperHUB iptables rules and routing
func CleanupRouting() {
	run("iptables", "-t", "nat", "-D", "POSTROUTING", "-j", "JAMPERHUB-NAT")
	run("iptables", "-t", "nat", "-F", "JAMPERHUB-NAT")
	run("iptables", "-t", "nat", "-X", "JAMPERHUB-NAT")

	run("iptables", "-D", "FORWARD", "-j", "JAMPERHUB-FWD")
	run("iptables", "-F", "JAMPERHUB-FWD")
	run("iptables", "-X", "JAMPERHUB-FWD")

	run("ip", "route", "flush", "table", routeTable)
	run("ip", "route", "flush", "table", "200")
	run("ip", "rule", "del", "from", "198.18.0.0/24", "lookup", routeTable)
	run("ip", "rule", "del", "fwmark", fwMark, "lookup", "200")

	log.Printf("[netutil] routing cleaned up")
}

// internals

func ensureRule(args ...string) {
	// check if rule exists
	out, _ := exec.Command("ip", "rule", "show").CombinedOutput()
	check := strings.Join(args, " ")
	if strings.Contains(string(out), check) {
		return
	}
	cmdArgs := append([]string{"rule", "add"}, args...)
	run("ip", cmdArgs...)
}

func getDefaultGateway() string {
	out, err := exec.Command("ip", "route", "show", "default").CombinedOutput()
	if err != nil {
		return ""
	}
	// "default via 10.0.1.1 dev enp6s18 ..."
	fields := strings.Fields(string(out))
	for i, f := range fields {
		if f == "via" && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	return ""
}

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %s: %w", name, args, strings.TrimSpace(string(out)), err)
	}
	return nil
}
