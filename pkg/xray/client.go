package xray

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Client manages an xray process + tun2socks process
type Client struct {
	id         string
	xrayBin    string
	tun2Bin    string
	dataDir    string
	configData string
	socksPort  int
	fwMark     string

	mu          sync.Mutex
	xrayCmd     *exec.Cmd
	tun2Cmd     *exec.Cmd
	running     bool
	iface       string // tun-jh-{id}
}

// NewClient creates a new Xray tunnel client.
// socksPort must be unique per instance (10808, 10809, ...).
// fwMark is used by tun2socks for loop prevention.
func NewClient(id, xrayBin, tun2Bin, dataDir, configData string, socksPort int, fwMark string) (*Client, error) {
	// validate JSON
	var raw map[string]any
	if err := json.Unmarshal([]byte(configData), &raw); err != nil {
		return nil, fmt.Errorf("xray: invalid config JSON: %w", err)
	}

	return &Client{
		id:         id,
		xrayBin:    xrayBin,
		tun2Bin:    tun2Bin,
		dataDir:    dataDir,
		configData: configData,
		socksPort:  socksPort,
		fwMark:     fwMark,
		iface:      "tun-jh-" + shortID(id),
	}, nil
}

// Start launches xray + tun2socks
func (c *Client) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return nil
	}

	// Step 1: write xray config with our SOCKS5 port
	confPath, err := c.writeConfig()
	if err != nil {
		return err
	}

	// Step 2: start xray
	c.xrayCmd = exec.Command(c.xrayBin, "run", "-c", confPath)
	c.xrayCmd.Stdout = os.Stdout
	c.xrayCmd.Stderr = os.Stderr
	if err := c.xrayCmd.Start(); err != nil {
		return fmt.Errorf("xray: start: %w", err)
	}

	// Step 3: wait for SOCKS5 port to be ready
	if err := waitPort(c.socksPort, 10*time.Second); err != nil {
		c.xrayCmd.Process.Kill()
		c.xrayCmd.Wait()
		return fmt.Errorf("xray: %w", err)
	}

	// Step 4: start tun2socks with fwmark for loop prevention
	c.tun2Cmd = exec.Command(c.tun2Bin,
		"-device", "tun://"+c.iface,
		"-proxy", fmt.Sprintf("socks5://127.0.0.1:%d", c.socksPort),
		"-fwmark", c.fwMark,
		"-loglevel", "warn",
	)
	c.tun2Cmd.Stdout = os.Stdout
	c.tun2Cmd.Stderr = os.Stderr
	if err := c.tun2Cmd.Start(); err != nil {
		c.xrayCmd.Process.Kill()
		c.xrayCmd.Wait()
		return fmt.Errorf("xray: start tun2socks: %w", err)
	}

	// Step 5: wait for tun interface, then bring it up
	if err := waitIface(c.iface, 5*time.Second); err != nil {
		c.cleanup()
		return fmt.Errorf("xray: %w", err)
	}

	if err := run("ip", "addr", "add", "198.18.255.1/30", "dev", c.iface); err != nil {
		c.cleanup()
		return fmt.Errorf("xray: ip addr: %w", err)
	}
	if err := run("ip", "link", "set", c.iface, "up"); err != nil {
		c.cleanup()
		return fmt.Errorf("xray: ip link up: %w", err)
	}

	c.running = true
	log.Printf("[xray] %s started on %s (socks5://127.0.0.1:%d)", c.id, c.iface, c.socksPort)

	// watch processes
	go c.watchProcess(c.xrayCmd, "xray")
	go c.watchProcess(c.tun2Cmd, "tun2socks")

	return nil
}

// Stop kills xray + tun2socks and cleans up
func (c *Client) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cleanup()
	log.Printf("[xray] %s stopped", c.id)
	return nil
}

func (c *Client) Running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

func (c *Client) Interface() string { return c.iface }

func (c *Client) SocksAddr() string {
	return fmt.Sprintf("socks5://127.0.0.1:%d", c.socksPort)
}

// internals

func (c *Client) writeConfig() (string, error) {
	// parse user config, override inbound to our SOCKS5 port
	config := c.patchInbound()

	dir := filepath.Join(c.dataDir, "xray")
	os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, c.id+".json")

	if err := os.WriteFile(path, []byte(config), 0644); err != nil {
		return "", fmt.Errorf("xray: write config: %w", err)
	}
	return path, nil
}

// patchInbound replaces inbound SOCKS5 listen port to our assigned port
func (c *Client) patchInbound() string {
	var cfg map[string]any
	if err := json.Unmarshal([]byte(c.configData), &cfg); err != nil {
		return c.configData // return as-is if can't parse
	}

	// replace or create inbound with our port
	inbound := map[string]any{
		"protocol": "socks",
		"listen":   "127.0.0.1",
		"port":     c.socksPort,
		"settings": map[string]any{"udp": true},
	}
	cfg["inbounds"] = []any{inbound}

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return c.configData
	}
	return string(out)
}

func (c *Client) cleanup() {
	if c.tun2Cmd != nil && c.tun2Cmd.Process != nil {
		c.tun2Cmd.Process.Kill()
		c.tun2Cmd.Wait()
	}
	if c.xrayCmd != nil && c.xrayCmd.Process != nil {
		c.xrayCmd.Process.Kill()
		c.xrayCmd.Wait()
	}
	run("ip", "link", "del", c.iface)
	c.running = false
}

func (c *Client) watchProcess(cmd *exec.Cmd, name string) {
	cmd.Wait()
	c.mu.Lock()
	wasRunning := c.running
	c.mu.Unlock()
	if wasRunning {
		log.Printf("[xray] %s %s process exited unexpectedly", c.id, name)
	}
}

func waitPort(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for time.Now().Before(deadline) {
		conn, err := (&net.Dialer{Timeout: 200 * time.Millisecond}).Dial("tcp", addr)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for port %d", port)
}

func waitIface(name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat("/sys/class/net/" + name); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for interface %s", name)
}

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %s: %w", name, args, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// shortID returns first 8 chars of tunnel ID for interface naming (max 15 chars for linux)
func shortID(id string) string {
	id = strings.TrimPrefix(id, "t-")
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
