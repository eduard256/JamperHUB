package dhcp

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/eduard256/jamperhub/pkg/config"
)

// Lease represents a DHCP lease
type Lease struct {
	IP       string `json:"ip"`
	MAC      string `json:"mac"`
	Hostname string `json:"hostname"`
	Expires  string `json:"expires"`
}

var (
	mu        sync.Mutex
	cmd       *exec.Cmd
	confPath  string
	leasePath string
)

func Init() {
	dataPath := config.DataPath()
	confPath = filepath.Join(dataPath, "dnsmasq.conf")
	leasePath = filepath.Join(dataPath, "dnsmasq.leases")
}

// Start generates dnsmasq config and starts the process
func Start(iface, gatewayIP string) error {
	mu.Lock()
	defer mu.Unlock()

	cfg := config.Get()
	if !cfg.DHCP.Enabled {
		log.Printf("[dhcp] disabled, skipping")
		return nil
	}

	if err := writeConfig(iface, gatewayIP, cfg.DHCP); err != nil {
		return err
	}

	return startProcess()
}

// Stop kills the dnsmasq process
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	stopProcess()
}

// Reload regenerates config and sends SIGHUP to dnsmasq
func Reload() {
	mu.Lock()
	defer mu.Unlock()

	cfg := config.Get()
	if !cfg.DHCP.Enabled {
		stopProcess()
		return
	}

	// re-read network config for interface/gateway
	iface := cfg.Network.Output
	gatewayIP := strings.Split(cfg.Network.Subnet, "/")[0]
	// replace last octet with 1 for gateway
	parts := strings.Split(gatewayIP, ".")
	if len(parts) == 4 {
		parts[3] = "1"
		gatewayIP = strings.Join(parts, ".")
	}

	if err := writeConfig(iface, gatewayIP, cfg.DHCP); err != nil {
		log.Printf("[dhcp] reload write config: %v", err)
		return
	}

	if cmd != nil && cmd.Process != nil {
		cmd.Process.Signal(syscall.SIGHUP)
		log.Printf("[dhcp] sent SIGHUP to dnsmasq")
	} else {
		startProcess()
	}
}

// GetLeases parses the dnsmasq lease file
func GetLeases() []Lease {
	data, err := os.ReadFile(leasePath)
	if err != nil {
		return []Lease{}
	}

	leases := make([]Lease, 0)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		// format: <expire_timestamp> <mac> <ip> <hostname> [client-id]
		expires := ""
		if ts, err := strconv.ParseInt(fields[0], 10, 64); err == nil {
			t := time.Unix(ts, 0)
			expires = t.Format(time.RFC3339)
		}

		hostname := fields[3]
		if hostname == "*" {
			hostname = ""
		}

		leases = append(leases, Lease{
			IP:       fields[2],
			MAC:      fields[1],
			Hostname: hostname,
			Expires:  expires,
		})
	}
	return leases
}

// internals

func writeConfig(iface, gatewayIP string, cfg config.DHCP) error {
	dns := strings.Join(cfg.DNS, ",")

	conf := fmt.Sprintf(`# JamperHUB dnsmasq config (auto-generated)
interface=%s
bind-interfaces
dhcp-range=%s,%s,%s
dhcp-option=3,%s
dhcp-option=6,%s
dhcp-leasefile=%s
no-resolv
server=%s
log-facility=-
`,
		iface,
		cfg.RangeStart, cfg.RangeEnd, cfg.LeaseTime,
		gatewayIP,
		dns,
		leasePath,
		strings.Join(cfg.DNS, "\nserver="),
	)

	if err := os.WriteFile(confPath, []byte(conf), 0644); err != nil {
		return fmt.Errorf("dhcp: write config: %w", err)
	}
	log.Printf("[dhcp] config written: %s", confPath)
	return nil
}

func startProcess() error {
	stopProcess()

	// check dnsmasq exists
	dnsmasqBin, err := exec.LookPath("dnsmasq")
	if err != nil {
		return fmt.Errorf("dhcp: dnsmasq not found, install with: sudo apt install dnsmasq")
	}

	cmd = exec.Command(dnsmasqBin,
		"--keep-in-foreground",
		"--conf-file="+confPath,
		"--no-daemon",
		"--log-queries",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("dhcp: start dnsmasq: %w", err)
	}

	log.Printf("[dhcp] dnsmasq started (pid %d)", cmd.Process.Pid)

	// watch in background
	go func() {
		err := cmd.Wait()
		mu.Lock()
		cmd = nil
		mu.Unlock()
		if err != nil {
			log.Printf("[dhcp] dnsmasq exited: %v", err)
		}
	}()

	return nil
}

func stopProcess() {
	if cmd != nil && cmd.Process != nil {
		cmd.Process.Kill()
		cmd.Wait()
		log.Printf("[dhcp] dnsmasq stopped")
	}
	cmd = nil
}
