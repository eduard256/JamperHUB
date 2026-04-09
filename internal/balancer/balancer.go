package balancer

import (
	"sync"
	"time"

	"github.com/eduard256/jamperhub/pkg/tunnel"
)

// SystemState represents the overall state of JamperHUB
type SystemState struct {
	SystemState  string       `json:"state"`  // "running", "waiting", "no_internet", "first_run"
	Uptime       int64        `json:"uptime"` // seconds
	InputIP      string       `json:"input_ip"`
	InputGateway string       `json:"input_gateway"`
	HasInternet  bool         `json:"has_internet"`
	OutputIP     string       `json:"output_ip"`
	ActiveTunnel *ActiveInfo  `json:"active_tunnel"`
	TunnelsUp    int          `json:"tunnels_up"`
	TunnelsDown  int          `json:"tunnels_down"`
	TunnelsTotal int          `json:"tunnels_total"`
	DHCPLeases   int          `json:"dhcp_leases"`
	Migration    *Migration   `json:"migration"`
}

type ActiveInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Latency int    `json:"latency"`
	Uptime  int64  `json:"uptime"`
}

// Migration holds the current migration state for the UI
type Migration struct {
	State             string      `json:"state"` // "evaluating", "waiting_for_idle", "switching", "completed", "cancelled", "cooldown"
	From              *MigrationEndpoint `json:"from,omitempty"`
	To                *MigrationEndpoint `json:"to,omitempty"`
	Reason            string      `json:"reason,omitempty"`
	ActiveConnections int         `json:"active_connections,omitempty"`
	WaitingSince      *time.Time  `json:"waiting_since,omitempty"`
	TimeoutAt         *time.Time  `json:"timeout_at,omitempty"`
	CompletedAt       *time.Time  `json:"completed_at,omitempty"`
	NextSwitchAt      *time.Time  `json:"next_switch_allowed_at,omitempty"`
	RemainingSeconds  int         `json:"remaining_seconds,omitempty"`
}

type MigrationEndpoint struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	SpeedMbps float64 `json:"speed_mbps,omitempty"`
}

var (
	mu        sync.RWMutex
	state     SystemState
	statuses  []tunnel.Status
	migration *Migration
	startTime = time.Now()
)

func Init() {
	state = SystemState{
		SystemState: "first_run",
	}
}

// GetState returns the current system state
func GetState() SystemState {
	mu.RLock()
	defer mu.RUnlock()
	s := state
	s.Uptime = int64(time.Since(startTime).Seconds())
	return s
}

// GetTunnelStatuses returns the status of all tunnels
func GetTunnelStatuses() []tunnel.Status {
	mu.RLock()
	defer mu.RUnlock()
	if statuses == nil {
		return []tunnel.Status{}
	}
	return statuses
}

// GetMigration returns the current migration state
func GetMigration() *Migration {
	mu.RLock()
	defer mu.RUnlock()
	return migration
}

// Reload is called when config changes, restarts balancer loop
func Reload() {
	// TODO: implement full reload logic
}

// StartTunnel starts a specific tunnel by ID
func StartTunnel(id string) {
	// TODO: implement
}

// StopTunnel stops a specific tunnel by ID
func StopTunnel(id string) {
	// TODO: implement
}

// RestartTunnel stops and starts a tunnel
func RestartTunnel(id string) {
	StopTunnel(id)
	StartTunnel(id)
}

// ActivateTunnel forces a tunnel to become active
func ActivateTunnel(id string) {
	// TODO: implement
}

// CancelMigration cancels a pending speed-based migration
func CancelMigration() {
	mu.Lock()
	migration = nil
	mu.Unlock()
}
