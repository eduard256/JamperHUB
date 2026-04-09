package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Config is the root configuration for JamperHUB
type Config struct {
	Network  Network  `json:"network"`
	DHCP     DHCP     `json:"dhcp"`
	Balancer Balancer `json:"balancer"`
	Servers  []Server `json:"servers"`
}

type Network struct {
	Input      string `json:"input"`
	Output     string `json:"output"`
	OutputMode string `json:"output_mode"` // "physical" or "bridge"
	Subnet     string `json:"subnet"`
}

type DHCP struct {
	Enabled    bool     `json:"enabled"`
	RangeStart string   `json:"range_start"`
	RangeEnd   string   `json:"range_end"`
	LeaseTime  string   `json:"lease_time"`
	DNS        []string `json:"dns"`
}

type Balancer struct {
	HealthcheckInterval  int     `json:"healthcheck_interval"`   // seconds
	SpeedTestInterval    int     `json:"speed_test_interval"`    // seconds
	TestURL              string  `json:"test_url"`
	SpeedTestURL         string  `json:"speed_test_url"`
	SpeedTestSize        int     `json:"speed_test_size"`        // bytes
	FallbackDirect       bool    `json:"fallback_direct"`
	SwitchThresholdPct   int     `json:"switch_threshold_percent"`
	SwitchCooldown       int     `json:"switch_cooldown"`        // seconds
	SwitchWaitTimeout    int     `json:"switch_wait_timeout"`    // seconds
	PingTimeout          int     `json:"ping_timeout"`           // seconds
	PingRetries          int     `json:"ping_retries"`
}

type Server struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Enabled    bool   `json:"enabled"`
	ConfigData string `json:"config_data"`
}

var (
	mu       sync.RWMutex
	current  Config
	dataPath string
)

// Default returns a config with sane defaults
func Default() Config {
	return Config{
		Network: Network{
			Subnet:     "198.18.0.0/24",
			OutputMode: "physical",
		},
		DHCP: DHCP{
			Enabled:    true,
			RangeStart: "198.18.0.100",
			RangeEnd:   "198.18.0.200",
			LeaseTime:  "12h",
			DNS:        []string{"8.8.8.8", "1.1.1.1"},
		},
		Balancer: Balancer{
			HealthcheckInterval: 10,
			SpeedTestInterval:   7200,
			TestURL:             "http://cp.cloudflare.com/generate_204",
			SpeedTestURL:        "https://speed.cloudflare.com/__down?bytes=3000000",
			SpeedTestSize:       3_000_000,
			FallbackDirect:      true,
			SwitchThresholdPct:  35,
			SwitchCooldown:      1800,
			SwitchWaitTimeout:   600,
			PingTimeout:         5,
			PingRetries:         3,
		},
	}
}

// Init sets the data path and loads config from disk, or creates default
func Init(path string) error {
	dataPath = path

	if err := os.MkdirAll(path, 0755); err != nil {
		return fmt.Errorf("config: mkdir %s: %w", path, err)
	}

	data, err := os.ReadFile(filePath())
	if err != nil {
		if os.IsNotExist(err) {
			current = Default()
			return Save()
		}
		return fmt.Errorf("config: read: %w", err)
	}

	cfg := Default() // start with defaults, overlay from file
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("config: parse: %w", err)
	}

	mu.Lock()
	current = cfg
	mu.Unlock()
	return nil
}

// Get returns a copy of the current config
func Get() Config {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// Set replaces the current config and saves to disk
func Set(cfg Config) error {
	mu.Lock()
	current = cfg
	mu.Unlock()
	return Save()
}

// Save writes config to disk atomically
func Save() error {
	mu.RLock()
	data, err := json.MarshalIndent(current, "", "  ")
	mu.RUnlock()
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}

	tmp := filePath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("config: write tmp: %w", err)
	}
	if err := os.Rename(tmp, filePath()); err != nil {
		return fmt.Errorf("config: rename: %w", err)
	}
	return nil
}

// SetNetwork updates only the network section
func SetNetwork(n Network) error {
	mu.Lock()
	current.Network = n
	mu.Unlock()
	return Save()
}

// SetDHCP updates only the DHCP section
func SetDHCP(d DHCP) error {
	mu.Lock()
	current.DHCP = d
	mu.Unlock()
	return Save()
}

// SetBalancer updates only the balancer section
func SetBalancer(b Balancer) error {
	mu.Lock()
	current.Balancer = b
	mu.Unlock()
	return Save()
}

// AddServer appends a server and saves
func AddServer(s Server) error {
	mu.Lock()
	current.Servers = append(current.Servers, s)
	mu.Unlock()
	return Save()
}

// UpdateServer replaces a server by ID
func UpdateServer(id string, fn func(*Server)) error {
	mu.Lock()
	for i := range current.Servers {
		if current.Servers[i].ID == id {
			fn(&current.Servers[i])
			mu.Unlock()
			return Save()
		}
	}
	mu.Unlock()
	return fmt.Errorf("config: server not found: %s", id)
}

// RemoveServer deletes a server by ID
func RemoveServer(id string) error {
	mu.Lock()
	for i, s := range current.Servers {
		if s.ID == id {
			current.Servers = append(current.Servers[:i], current.Servers[i+1:]...)
			mu.Unlock()
			return Save()
		}
	}
	mu.Unlock()
	return fmt.Errorf("config: server not found: %s", id)
}

// GetServer returns a server by ID
func GetServer(id string) (Server, bool) {
	mu.RLock()
	defer mu.RUnlock()
	for _, s := range current.Servers {
		if s.ID == id {
			return s, true
		}
	}
	return Server{}, false
}

// IsFirstRun returns true if no input interface is configured
func IsFirstRun() bool {
	mu.RLock()
	defer mu.RUnlock()
	return current.Network.Input == ""
}

// DataPath returns the data directory path
func DataPath() string {
	return dataPath
}

func filePath() string {
	return filepath.Join(dataPath, "config.json")
}
