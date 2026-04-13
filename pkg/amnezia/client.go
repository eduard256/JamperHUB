package amnezia

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Client manages an amneziawg-go process and its TUN interface
type Client struct {
	id      string
	binPath string
	iface   string
	cfg     *Config

	mu      sync.Mutex
	cmd     *exec.Cmd
	running bool
}

// NewClient creates a new Amnezia tunnel client
func NewClient(id, binPath, configData string) (*Client, error) {
	cfg, err := ParseConfig(configData)
	if err != nil {
		return nil, err
	}
	return &Client{
		id:      id,
		binPath: binPath,
		iface:   "awg-" + shortID(id),
		cfg:     cfg,
	}, nil
}

// Start launches amneziawg-go, configures via UAPI, sets up networking
func (c *Client) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return nil
	}

	// Step 0: cleanup stale socket and interface from previous run
	os.Remove(fmt.Sprintf("%s/%s.sock", socketDir, c.iface))
	run("ip", "link", "del", c.iface)

	// Step 1: start amneziawg-go in foreground
	c.cmd = exec.Command(c.binPath, "-f", c.iface)
	c.cmd.Stdout = os.Stdout
	c.cmd.Stderr = os.Stderr

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("amnezia: start process: %w", err)
	}

	// Step 2: wait for UAPI socket
	if err := c.waitSocket(5 * time.Second); err != nil {
		c.cmd.Process.Kill()
		c.cmd.Wait()
		return err
	}

	// Step 3: configure via UAPI
	if err := c.configure(); err != nil {
		c.cmd.Process.Kill()
		c.cmd.Wait()
		return err
	}

	// Step 4: set up IP and bring interface up
	if err := c.setupNetwork(); err != nil {
		c.cmd.Process.Kill()
		c.cmd.Wait()
		return err
	}

	c.running = true
	log.Printf("[amnezia] %s started on %s -> %s", c.id, c.iface, c.cfg.Endpoint)

	// watch process in background, mark as not running on exit
	go func() {
		c.cmd.Wait()
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
		log.Printf("[amnezia] %s process exited", c.id)
	}()

	return nil
}

// Stop kills the amneziawg-go process and cleans up
func (c *Client) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running || c.cmd == nil || c.cmd.Process == nil {
		c.running = false
		return nil
	}

	c.cmd.Process.Kill()
	c.cmd.Wait()
	c.running = false

	// remove interface (may already be gone after process death)
	run("ip", "link", "del", c.iface)

	log.Printf("[amnezia] %s stopped", c.id)
	return nil
}

func (c *Client) Running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

func (c *Client) Interface() string { return c.iface }

// GetStatus returns peer status via UAPI (for healthcheck)
func (c *Client) GetStatus() (*PeerStatus, error) {
	uapi, err := Dial(c.iface)
	if err != nil {
		return nil, err
	}
	defer uapi.Close()
	return uapi.GetPeerStatus()
}

// internals

func (c *Client) waitSocket(timeout time.Duration) error {
	path := fmt.Sprintf("%s/%s.sock", socketDir, c.iface)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(path); err == nil && info.Mode().Type() == os.ModeSocket {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("amnezia: timeout waiting for socket %s", path)
}

func (c *Client) configure() error {
	uapi, err := Dial(c.iface)
	if err != nil {
		return err
	}
	defer uapi.Close()

	setConf, err := c.cfg.BuildUAPI()
	if err != nil {
		return err
	}
	return uapi.Set(setConf)
}

func (c *Client) setupNetwork() error {
	// set address from config (e.g. "10.8.1.4/32")
	addr := c.cfg.Address
	if !strings.Contains(addr, "/") {
		addr += "/32"
	}
	if err := run("ip", "addr", "add", addr, "dev", c.iface); err != nil {
		return fmt.Errorf("amnezia: ip addr add: %w", err)
	}

	if err := run("ip", "link", "set", c.iface, "up"); err != nil {
		return fmt.Errorf("amnezia: ip link set up: %w", err)
	}

	return nil
}

// shortID returns first 8 chars for interface naming (Linux limit 15 chars: "awg-" + 8 = 12)
func shortID(id string) string {
	id = strings.TrimPrefix(id, "t-")
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %s: %w", name, args, strings.TrimSpace(string(out)), err)
	}
	return nil
}
