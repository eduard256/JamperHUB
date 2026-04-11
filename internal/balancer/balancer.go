package balancer

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/eduard256/jamperhub/internal/dhcp"
	"github.com/eduard256/jamperhub/internal/store"
	"github.com/eduard256/jamperhub/pkg/config"
	"github.com/eduard256/jamperhub/pkg/healthcheck"
	"github.com/eduard256/jamperhub/pkg/netutil"
	"github.com/eduard256/jamperhub/pkg/tunnel"
)

// tunnelState holds runtime state for a single tunnel
type tunnelState struct {
	id        string
	client    tunnel.Client
	cfg       config.Server
	priority  int // 1, 2, 3
	latency   int // ms, -1 = dead
	speedMbps float64
	failures  int
	startedAt time.Time
	lastCheck time.Time
	lastSpeed time.Time
	rxBytes   int64
	txBytes   int64
}

// SystemState for API
type SystemState struct {
	SystemState  string      `json:"state"`
	Uptime       int64       `json:"uptime"`
	InputIP      string      `json:"input_ip"`
	InputGateway string      `json:"input_gateway"`
	HasInternet  bool        `json:"has_internet"`
	OutputIP     string      `json:"output_ip"`
	ActiveTunnel *ActiveInfo `json:"active_tunnel"`
	TunnelsUp    int         `json:"tunnels_up"`
	TunnelsDown  int         `json:"tunnels_down"`
	TunnelsTotal int         `json:"tunnels_total"`
	DHCPLeases   int         `json:"dhcp_leases"`
	Migration    *Migration  `json:"migration"`
}

type ActiveInfo struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Type    string  `json:"type"`
	Latency int     `json:"latency"`
	Uptime  int64   `json:"uptime"`
	Speed   float64 `json:"speed_mbps"`
}

type Migration struct {
	State             string      `json:"state"`
	From              *MigEndpoint `json:"from,omitempty"`
	To                *MigEndpoint `json:"to,omitempty"`
	Reason            string      `json:"reason,omitempty"`
	ActiveConnections int         `json:"active_connections,omitempty"`
	WaitingSince      *time.Time  `json:"waiting_since,omitempty"`
	TimeoutAt         *time.Time  `json:"timeout_at,omitempty"`
	CompletedAt       *time.Time  `json:"completed_at,omitempty"`
	NextSwitchAt      *time.Time  `json:"next_switch_allowed_at,omitempty"`
	RemainingSeconds  int         `json:"remaining_seconds,omitempty"`
}

type MigEndpoint struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Speed float64 `json:"speed_mbps,omitempty"`
}

var (
	mu         sync.RWMutex
	tunnels    = map[string]*tunnelState{}
	activeID   string
	migration  *Migration
	lastSwitch time.Time
	startTime  = time.Now()
	hasNet     bool
	stopCh     chan struct{}
)

const (
	maxFailures       = 3
	idleThresholdKBps = 10.0
	idleSamples       = 3
)

var started bool // true after goroutines are launched

func Init() {
	stopCh = make(chan struct{})

	cfg := config.Get()
	if config.IsFirstRun() {
		log.Printf("[balancer] first run, waiting for setup")
		return
	}

	boot(cfg)
}

// Reload re-reads config and re-initializes.
// Also starts goroutines if this is the first real config (after setup wizard).
func Reload() {
	mu.Lock()
	// stop all tunnels
	for _, ts := range tunnels {
		if ts.client != nil && ts.client.Running() {
			ts.client.Stop()
		}
	}
	tunnels = map[string]*tunnelState{}
	activeID = ""
	mu.Unlock()

	cfg := config.Get()
	if cfg.Network.Input == "" {
		return
	}

	boot(cfg)
}

func boot(cfg config.Config) {
	setup(cfg)

	// if internet already available, start tunnels immediately
	if cfg.Network.Input != "" && netutil.HasInternet(cfg.Network.Input) {
		hasNet = true
		log.Printf("[balancer] internet available on %s, starting tunnels", cfg.Network.Input)
		go startAllTunnels()
	}

	// launch goroutines only once
	if !started {
		started = true
		go networkWatcher()
		go pingLoop()
		go speedTestLoop()
	}
}

// StartTunnel starts a specific tunnel by ID
func StartTunnel(id string) {
	cfg, ok := config.GetServer(id)
	if !ok || !cfg.Enabled {
		return
	}
	startOne(cfg)
}

// StopTunnel stops a specific tunnel by ID
func StopTunnel(id string) {
	mu.Lock()
	ts, ok := tunnels[id]
	if ok && ts.client != nil {
		ts.client.Stop()
		delete(tunnels, id)
		if activeID == id {
			activeID = ""
		}
	}
	mu.Unlock()

	if ok {
		store.AddEvent("info", "tunnel_down", id, "stopped by user", 0)
		// pick new active
		selectBest()
	}
}

// RestartTunnel stops and starts a tunnel
func RestartTunnel(id string) {
	StopTunnel(id)
	StartTunnel(id)
}

// ActivateTunnel forces a tunnel to become active
func ActivateTunnel(id string) {
	mu.Lock()
	ts, ok := tunnels[id]
	if !ok || ts.client == nil || !ts.client.Running() {
		mu.Unlock()
		return
	}
	prev := activeID
	activeID = id
	mu.Unlock()

	netutil.SetActiveRoute(tunnel.RoutingInterface(ts.client))
	store.AddEvent("info", "activate", id, "manually activated (was "+prev+")", 0)
	log.Printf("[balancer] manually activated %s", id)
}

// CancelMigration cancels a pending speed-based migration
func CancelMigration() {
	mu.Lock()
	migration = nil
	mu.Unlock()
}

// GetState returns current system state for API
func GetState() SystemState {
	mu.RLock()
	defer mu.RUnlock()

	cfg := config.Get()
	s := SystemState{
		SystemState:  systemState(),
		Uptime:       int64(time.Since(startTime).Seconds()),
		InputIP:      getIfaceIP(cfg.Network.Input),
		InputGateway: netutil.GetDefaultGW(),
		HasInternet:  hasNet,
		OutputIP:     getIfaceIP(cfg.Network.Output),
		TunnelsTotal: len(tunnels),
		DHCPLeases:   len(dhcp.GetLeases()),
		Migration:    migration,
	}

	for _, ts := range tunnels {
		if ts.priority == 3 && (ts.client == nil || !ts.client.Running()) {
			continue // standby -- don't count as up or down
		}
		if ts.client != nil && ts.client.Running() && ts.latency >= 0 {
			s.TunnelsUp++
		} else {
			s.TunnelsDown++
		}
	}

	if activeID != "" {
		if ts, ok := tunnels[activeID]; ok {
			s.ActiveTunnel = &ActiveInfo{
				ID:      ts.id,
				Name:    ts.cfg.Name,
				Type:    ts.cfg.Type,
				Latency: ts.latency,
				Uptime:  int64(time.Since(ts.startedAt).Seconds()),
				Speed:   ts.speedMbps,
			}
		}
	}

	return s
}

// GetTunnelStatuses returns status of all tunnels for API
func GetTunnelStatuses() []tunnel.Status {
	mu.RLock()
	defer mu.RUnlock()

	result := make([]tunnel.Status, 0, len(tunnels))
	for _, ts := range tunnels {
		st := tunnel.Status{
			ID:       ts.id,
			Name:     ts.cfg.Name,
			Type:     ts.cfg.Type,
			Enabled:  ts.cfg.Enabled,
			Priority: ts.priority,
		}

		if ts.client == nil {
			if ts.priority == 3 {
				st.State = tunnel.StateStandby
			} else {
				st.State = tunnel.StateDisabled
			}
		} else if !ts.client.Running() {
			st.State = tunnel.StateConnecting
		} else if ts.latency < 0 {
			st.State = tunnel.StateDown
		} else if ts.id == activeID {
			st.State = tunnel.StateActive
		} else {
			st.State = tunnel.StateReady
		}

		if ts.latency >= 0 {
			l := ts.latency
			st.Latency = &l
		}
		if ts.speedMbps > 0 {
			sp := ts.speedMbps
			st.SpeedMbps = &sp
		}
		if !ts.startedAt.IsZero() {
			st.Uptime = int64(time.Since(ts.startedAt).Seconds())
		}
		if !ts.lastSpeed.IsZero() {
			t := ts.lastSpeed
			st.LastSpeedTest = &t
		}

		st.TrafficIn = ts.rxBytes
		st.TrafficOut = ts.txBytes
		if ts.client != nil {
			st.Interface = ts.client.Interface()
			st.Mode = modeName(ts.client.Mode())
		}

		result = append(result, st)
	}
	return result
}

// GetMigration returns current migration state
func GetMigration() *Migration {
	mu.RLock()
	defer mu.RUnlock()
	return migration
}

// internals

func setup(cfg config.Config) {
	// setup output interface
	subnet := cfg.Network.Subnet
	gatewayIP := subnetToGateway(subnet)
	cidr := gatewayIP + "/" + subnetMask(subnet)

	if cfg.Network.Output != "" {
		if err := netutil.SetupOutputInterface(cfg.Network.Output, cidr); err != nil {
			log.Printf("[balancer] output iface: %v", err)
		}
	}

	netutil.EnableForwarding()

	if cfg.Network.Output != "" {
		netutil.SetupNAT(cfg.Network.Input, subnet)
		netutil.SetupForwarding(cfg.Network.Output)
	}

	if cfg.Network.Input != "" {
		netutil.SetupTun2socksRouting(cfg.Network.Input)
	}

	// start DHCP
	if cfg.Network.Output != "" {
		if err := dhcp.Start(cfg.Network.Output, gatewayIP); err != nil {
			log.Printf("[balancer] dhcp: %v", err)
		}
	}
}

func networkWatcher() {
	debounce := 0
	for {
		select {
		case <-stopCh:
			return
		case <-time.After(5 * time.Second):
		}

		cfg := config.Get()
		if cfg.Network.Input == "" {
			continue
		}

		internet := netutil.HasInternet(cfg.Network.Input)

		mu.Lock()
		wasOnline := hasNet
		hasNet = internet
		mu.Unlock()

		if internet && !wasOnline {
			debounce++
			if debounce < 2 {
				continue // debounce: wait one more cycle
			}
			debounce = 0
			log.Printf("[balancer] internet detected on %s", cfg.Network.Input)
			store.AddEvent("info", "network_up", "", "internet detected on "+cfg.Network.Input, 0)
			startAllTunnels()
		} else if !internet && wasOnline {
			debounce++
			if debounce < 2 {
				continue
			}
			debounce = 0
			log.Printf("[balancer] internet lost on %s", cfg.Network.Input)
			store.AddEvent("warn", "network_down", "", "internet lost on "+cfg.Network.Input, 0)
			stopAllTunnels()
		} else {
			debounce = 0
		}
	}
}

func pingLoop() {
	// wait for first tunnel to start
	time.Sleep(15 * time.Second)

	for {
		select {
		case <-stopCh:
			return
		case <-time.After(getDuration("healthcheck_interval", 10)):
		}

		mu.RLock()
		if !hasNet {
			mu.RUnlock()
			continue
		}
		ids := make([]string, 0, len(tunnels))
		for id := range tunnels {
			ids = append(ids, id)
		}
		mu.RUnlock()

		cfg := config.Get()
		testURL := cfg.Balancer.TestURL
		timeout := time.Duration(cfg.Balancer.PingTimeout) * time.Second

		for _, id := range ids {
			mu.RLock()
			ts, ok := tunnels[id]
			if !ok || ts.client == nil || !ts.client.Running() {
				mu.RUnlock()
				continue
			}
			iface := ts.client.Interface()
			isTUN := ts.client.Mode() == tunnel.ModeTUN
			mu.RUnlock()

			// extract socks port from interface string "socks5://127.0.0.1:PORT"
			socksPort := 0
			if !isTUN {
				fmt.Sscanf(iface, "socks5://127.0.0.1:%d", &socksPort)
			}

			latency, err := healthcheck.PingViaCurl(testURL, iface, isTUN, socksPort, timeout)

			mu.Lock()
			ts, ok = tunnels[id]
			if !ok {
				mu.Unlock()
				continue
			}
			ts.lastCheck = time.Now()

			if err != nil {
				ts.failures++
				if ts.failures >= maxFailures && ts.latency >= 0 {
					ts.latency = -1
					log.Printf("[balancer] %s DOWN (%d failures)", id, ts.failures)
					store.AddEvent("warn", "tunnel_down", id, "healthcheck failed "+fmt.Sprintf("%dx", ts.failures), 0)

					if activeID == id {
						mu.Unlock()
						selectBest()
						continue
					}
				}
			} else {
				if ts.latency < 0 {
					log.Printf("[balancer] %s UP (latency %dms)", id, latency)
					store.AddEvent("info", "tunnel_up", id, fmt.Sprintf("connected, latency %dms", latency), 0)
				}
				ts.failures = 0
				ts.latency = latency
			}

			// read interface traffic stats
			tunIface := tunnel.RoutingInterface(ts.client)
			rx := readIfaceStat(tunIface, "rx_bytes")
			tx := readIfaceStat(tunIface, "tx_bytes")
			ts.rxBytes = rx
			ts.txBytes = tx

			store.AddTrafficPoint(id, rx, tx, intPtr(ts.latency), stateStr(id))
			mu.Unlock()
		}

		// check if we need to switch active
		selectBest()
	}
}

func speedTestLoop() {
	// wait for tunnels to connect before first speed test
	time.Sleep(20 * time.Second)

	for {
		// run speed test immediately on first iteration, then wait interval

		mu.RLock()
		if !hasNet {
			mu.RUnlock()
			continue
		}
		ids := make([]string, 0, len(tunnels))
		for id := range tunnels {
			ids = append(ids, id)
		}
		mu.RUnlock()

		cfg := config.Get()
		testURL := cfg.Balancer.SpeedTestURL
		timeout := 30 * time.Second

		for _, id := range ids {
			mu.RLock()
			ts, ok := tunnels[id]
			if !ok || ts.client == nil || !ts.client.Running() || ts.latency < 0 {
				mu.RUnlock()
				continue
			}
			iface := ts.client.Interface()
			isTUN := ts.client.Mode() == tunnel.ModeTUN
			mu.RUnlock()

			socksPort := 0
			if !isTUN {
				fmt.Sscanf(iface, "socks5://127.0.0.1:%d", &socksPort)
			}

			speed, err := healthcheck.SpeedTest(testURL, iface, isTUN, socksPort, timeout)

			mu.Lock()
			if ts, ok := tunnels[id]; ok {
				ts.lastSpeed = time.Now()
				if err == nil {
					ts.speedMbps = speed
					store.AddSpeedPoint(id, speed)
					log.Printf("[balancer] speed test %s: %.1f Mbps", id, speed)
				} else {
					log.Printf("[balancer] speed test %s: %v", id, err)
				}
			}
			mu.Unlock()
		}

		// check for speed-based migration
		checkSpeedMigration()

		// wait for next cycle
		select {
		case <-stopCh:
			return
		case <-time.After(getDuration("speed_test_interval", 900)):
		}
	}
}

func checkSpeedMigration() {
	mu.Lock()
	defer mu.Unlock()

	if migration != nil {
		return // already migrating
	}

	cfg := config.Get()
	cooldown := time.Duration(cfg.Balancer.SwitchCooldown) * time.Second
	if time.Since(lastSwitch) < cooldown {
		return
	}

	if activeID == "" {
		return
	}
	activeTunnel, ok := tunnels[activeID]
	if !ok || activeTunnel.speedMbps <= 0 {
		return
	}

	// find fastest tunnel within the same priority level
	var bestID string
	var bestSpeed float64
	for id, ts := range tunnels {
		if id == activeID || ts.latency < 0 || ts.speedMbps <= 0 {
			continue
		}
		if ts.priority != activeTunnel.priority {
			continue // speed migration only within same priority
		}
		if ts.speedMbps > bestSpeed {
			bestSpeed = ts.speedMbps
			bestID = id
		}
	}

	if bestID == "" {
		return
	}

	threshold := float64(cfg.Balancer.SwitchThresholdPct) / 100.0
	diff := (bestSpeed - activeTunnel.speedMbps) / activeTunnel.speedMbps
	if diff < threshold {
		return
	}

	bestTunnel := tunnels[bestID]
	reason := fmt.Sprintf("%s is %.0f%% faster (%.1f vs %.1f Mbps)",
		bestTunnel.cfg.Name, diff*100, bestSpeed, activeTunnel.speedMbps)

	migration = &Migration{
		State:  "evaluating",
		From:   &MigEndpoint{ID: activeID, Name: activeTunnel.cfg.Name, Speed: activeTunnel.speedMbps},
		To:     &MigEndpoint{ID: bestID, Name: bestTunnel.cfg.Name, Speed: bestSpeed},
		Reason: reason,
	}

	log.Printf("[balancer] migration candidate: %s", reason)

	// start idle watcher in background
	go idleWatcher(bestID)
}

func idleWatcher(targetID string) {
	cfg := config.Get()
	waitTimeout := time.Duration(cfg.Balancer.SwitchWaitTimeout) * time.Second
	waitStart := time.Now()
	waitT := waitStart
	timeoutAt := waitStart.Add(waitTimeout)

	mu.Lock()
	if migration != nil {
		migration.State = "waiting_for_idle"
		migration.WaitingSince = &waitStart
		migration.TimeoutAt = &timeoutAt
	}
	mu.Unlock()

	var prevBytes int64
	idleCount := 0

	for {
		time.Sleep(1 * time.Second)

		mu.RLock()
		if migration == nil {
			mu.RUnlock()
			return // cancelled
		}
		if activeID == "" {
			mu.RUnlock()
			return
		}
		ts, ok := tunnels[activeID]
		if !ok || ts.client == nil {
			mu.RUnlock()
			return
		}
		iface := tunnel.RoutingInterface(ts.client)
		mu.RUnlock()

		// check timeout
		if time.Since(waitStart) > waitTimeout {
			mu.Lock()
			migration = &Migration{
				State:  "cancelled",
				Reason: "wait timeout, traffic still active",
			}
			mu.Unlock()
			log.Printf("[balancer] migration cancelled: timeout")
			time.Sleep(5 * time.Second)
			mu.Lock()
			migration = nil
			mu.Unlock()
			return
		}

		// check traffic rate
		now := time.Now()
		currentBytes, kbps := healthcheck.GetTrafficRate(iface, prevBytes, now.Sub(waitT))
		prevBytes = currentBytes
		waitT = now

		mu.Lock()
		if migration != nil {
			migration.ActiveConnections = int(kbps)
		}
		mu.Unlock()

		if kbps < idleThresholdKBps {
			idleCount++
		} else {
			idleCount = 0
		}

		if idleCount >= idleSamples {
			// idle! switch now
			doMigration(targetID)
			return
		}
	}
}

func doMigration(targetID string) {
	mu.Lock()
	ts, ok := tunnels[targetID]
	if !ok || ts.client == nil || !ts.client.Running() {
		migration = nil
		mu.Unlock()
		return
	}

	if migration != nil {
		migration.State = "switching"
	}

	prevActive := activeID
	activeID = targetID
	iface := tunnel.RoutingInterface(ts.client)
	mu.Unlock()

	netutil.SetActiveRoute(iface)

	now := time.Now()
	mu.Lock()
	lastSwitch = now
	nextSwitch := now.Add(time.Duration(config.Get().Balancer.SwitchCooldown) * time.Second)
	migration = &Migration{
		State:        "completed",
		CompletedAt:  &now,
		NextSwitchAt: &nextSwitch,
	}
	mu.Unlock()

	store.AddEvent("info", "failover", targetID,
		fmt.Sprintf("speed migration: %s -> %s", prevActive, targetID), 0)
	log.Printf("[balancer] migrated %s -> %s", prevActive, targetID)

	// clear migration after 10 seconds
	time.Sleep(10 * time.Second)
	mu.Lock()
	migration = nil
	mu.Unlock()
}

func startAllTunnels() {
	cfg := config.Get()
	// start priority 1 and 2 only; priority 3 stays in standby
	for _, srv := range cfg.Servers {
		if srv.Enabled && srv.Priority != 3 {
			startOne(srv)
		}
	}
	// register priority 3 as standby (not started)
	for _, srv := range cfg.Servers {
		if srv.Enabled && srv.Priority == 3 {
			registerStandby(srv)
		}
	}

	// wait for tunnels to connect, then select best
	time.Sleep(5 * time.Second)
	selectBest()
}

func stopAllTunnels() {
	mu.Lock()
	for _, ts := range tunnels {
		if ts.client != nil && ts.client.Running() {
			ts.client.Stop()
		}
	}
	tunnels = map[string]*tunnelState{}
	activeID = ""
	mu.Unlock()

	// set fallback direct
	cfg := config.Get()
	gw := netutil.GetDefaultGW()
	netutil.SetDirectRoute(cfg.Network.Input, gw)
}

func startOne(srv config.Server) {
	mu.Lock()
	if _, exists := tunnels[srv.ID]; exists {
		mu.Unlock()
		return
	}
	mu.Unlock()

	client := tunnel.NewClient(srv.Type, srv.ID, srv.ConfigData)
	if client == nil {
		log.Printf("[balancer] unknown tunnel type: %s", srv.Type)
		return
	}

	if err := client.Start(); err != nil {
		log.Printf("[balancer] start %s: %v", srv.ID, err)
		store.AddEvent("error", "tunnel_error", srv.ID, err.Error(), 0)
		return
	}

	priority := srv.Priority
	if priority < 1 || priority > 3 {
		priority = 2
	}

	mu.Lock()
	tunnels[srv.ID] = &tunnelState{
		id:        srv.ID,
		client:    client,
		cfg:       srv,
		priority:  priority,
		latency:   -1,
		startedAt: time.Now(),
	}
	mu.Unlock()

	store.AddEvent("info", "tunnel_up", srv.ID, "started "+srv.Name, 0)
}

// registerStandby adds a priority 3 tunnel to the map without starting it
func registerStandby(srv config.Server) {
	mu.Lock()
	if _, exists := tunnels[srv.ID]; exists {
		mu.Unlock()
		return
	}
	tunnels[srv.ID] = &tunnelState{
		id:       srv.ID,
		client:   nil, // not started
		cfg:      srv,
		priority: 3,
		latency:  -1,
	}
	mu.Unlock()
}

// startPriority3 launches priority 3 tunnels two at a time until one connects
func startPriority3() {
	mu.RLock()
	var standby []string
	for id, ts := range tunnels {
		if ts.priority == 3 && ts.client == nil {
			standby = append(standby, id)
		}
	}
	mu.RUnlock()

	if len(standby) == 0 {
		return
	}

	log.Printf("[balancer] starting priority 3 tunnels (%d available)", len(standby))
	store.AddEvent("warn", "priority3_start", "", fmt.Sprintf("starting backup tunnels, %d available", len(standby)), 0)

	// launch two at a time
	for i := 0; i < len(standby); i += 2 {
		batch := standby[i:]
		if len(batch) > 2 {
			batch = batch[:2]
		}

		for _, id := range batch {
			mu.RLock()
			ts, ok := tunnels[id]
			mu.RUnlock()
			if !ok {
				continue
			}
			go startOne(ts.cfg)
		}

		// wait for one to connect
		time.Sleep(10 * time.Second)

		// check if any connected
		mu.RLock()
		for _, id := range batch {
			if ts, ok := tunnels[id]; ok && ts.client != nil && ts.client.Running() && ts.latency >= 0 {
				mu.RUnlock()
				log.Printf("[balancer] priority 3 tunnel %s connected", id)
				return
			}
		}
		mu.RUnlock()
	}
}

// stopPriority3 stops all running priority 3 tunnels
func stopPriority3() {
	mu.Lock()
	for id, ts := range tunnels {
		if ts.priority == 3 && ts.client != nil && ts.client.Running() {
			ts.client.Stop()
			ts.client = nil
			ts.latency = -1
			ts.startedAt = time.Time{}
			log.Printf("[balancer] stopped priority 3 tunnel %s", id)
			store.AddEvent("info", "tunnel_down", id, "backup tunnel stopped, higher priority available", 0)
		}
	}
	mu.Unlock()
}

// Rebalance re-evaluates active tunnel based on current priorities (called on priority change)
func Rebalance() {
	// sync priorities from config to runtime state
	mu.Lock()
	cfg := config.Get()
	for _, srv := range cfg.Servers {
		if ts, ok := tunnels[srv.ID]; ok {
			p := srv.Priority
			if p < 1 || p > 3 {
				p = 2
			}
			ts.priority = p
			ts.cfg = srv
		}
	}
	mu.Unlock()

	selectBest()
}

func selectBest() {
	mu.Lock()
	defer mu.Unlock()

	type candidate struct {
		id       string
		priority int
		latency  int
	}

	var alive []candidate
	for id, ts := range tunnels {
		if ts.client != nil && ts.client.Running() && ts.latency >= 0 {
			alive = append(alive, candidate{id, ts.priority, ts.latency})
		}
	}

	// no alive tunnels at priority 1 or 2 -> start priority 3
	hasP1P2 := false
	for _, c := range alive {
		if c.priority <= 2 {
			hasP1P2 = true
			break
		}
	}

	if len(alive) == 0 {
		if activeID != "" {
			// check if there are priority 3 tunnels to try
			hasP3 := false
			for _, ts := range tunnels {
				if ts.priority == 3 && ts.client == nil {
					hasP3 = true
					break
				}
			}
			if hasP3 {
				mu.Unlock()
				startPriority3()
				mu.Lock()
				// re-check after starting p3
				for id, ts := range tunnels {
					if ts.client != nil && ts.client.Running() && ts.latency >= 0 {
						alive = append(alive, candidate{id, ts.priority, ts.latency})
					}
				}
			}
		}

		if len(alive) == 0 {
			if activeID != "" {
				activeID = ""
				log.Printf("[balancer] all tunnels down, fallback direct")
				store.AddEvent("critical", "all_down", "", "all tunnels down, fallback to direct", 0)
				cfg := config.Get()
				gw := netutil.GetDefaultGW()
				go netutil.SetDirectRoute(cfg.Network.Input, gw)
			}
			return
		}
	}

	// if we have priority 1/2 alive and priority 3 running -> stop priority 3
	if hasP1P2 {
		for _, ts := range tunnels {
			if ts.priority == 3 && ts.client != nil && ts.client.Running() {
				go stopPriority3()
				break
			}
		}
	}

	// find best candidate: prefer highest priority (lowest number), then lowest latency
	sort.Slice(alive, func(i, j int) bool {
		if alive[i].priority != alive[j].priority {
			return alive[i].priority < alive[j].priority
		}
		return alive[i].latency < alive[j].latency
	})

	best := alive[0]

	if activeID == best.id {
		return // already active
	}

	// if we have an active tunnel, check if switch is needed
	if activeID != "" {
		activeTunnel, ok := tunnels[activeID]
		if ok && activeTunnel.latency >= 0 {
			// current is alive -- only switch if best has higher priority (lower number)
			if best.priority >= activeTunnel.priority {
				return // same or lower priority, don't switch (speed-based handled by speedTestLoop)
			}
			// best has higher priority -- switch (wait for idle handled by caller for speed-based)
		}
	}

	prev := activeID
	activeID = best.id
	ts := tunnels[best.id]

	go netutil.SetActiveRoute(tunnel.RoutingInterface(ts.client))

	if prev != "" {
		log.Printf("[balancer] failover %s -> %s (priority %d, latency %dms)", prev, best.id, best.priority, best.latency)
		store.AddEvent("info", "failover", best.id,
			fmt.Sprintf("failover %s -> %s (priority %d)", prev, best.id, best.priority),
			int(time.Since(ts.lastCheck).Milliseconds()))
	} else {
		log.Printf("[balancer] active: %s (priority %d, latency %dms)", best.id, best.priority, best.latency)
	}
}

// helpers

func getDuration(key string, defaultSec int) time.Duration {
	cfg := config.Get()
	switch key {
	case "healthcheck_interval":
		if cfg.Balancer.HealthcheckInterval > 0 {
			return time.Duration(cfg.Balancer.HealthcheckInterval) * time.Second
		}
	case "speed_test_interval":
		if cfg.Balancer.SpeedTestInterval > 0 {
			return time.Duration(cfg.Balancer.SpeedTestInterval) * time.Second
		}
	}
	return time.Duration(defaultSec) * time.Second
}

func systemState() string {
	if config.IsFirstRun() {
		return "first_run"
	}
	if !hasNet {
		return "waiting"
	}
	if len(tunnels) == 0 {
		return "no_tunnels"
	}
	if activeID == "" {
		return "all_down"
	}
	return "running"
}

func getIfaceIP(name string) string {
	ifaces, _ := netutil.ListInterfaces()
	for _, i := range ifaces {
		if i.Name == name && i.IP != nil {
			return *i.IP
		}
	}
	return ""
}

func subnetToGateway(subnet string) string {
	parts := strings.Split(subnet, "/")
	ip := parts[0]
	octets := strings.Split(ip, ".")
	if len(octets) == 4 {
		octets[3] = "1"
		return strings.Join(octets, ".")
	}
	return ip
}

func subnetMask(subnet string) string {
	parts := strings.Split(subnet, "/")
	if len(parts) == 2 {
		return parts[1]
	}
	return "24"
}

func modeName(m tunnel.Mode) string {
	if m == tunnel.ModeTUN {
		return "tun"
	}
	return "proxy"
}

func stateStr(id string) string {
	if id == activeID {
		return "active"
	}
	return "ready"
}

func intPtr(v int) *int {
	if v < 0 {
		return nil
	}
	return &v
}

func readIfaceStat(iface, stat string) int64 {
	data, err := os.ReadFile("/sys/class/net/" + iface + "/statistics/" + stat)
	if err != nil {
		return 0
	}
	var val int64
	fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &val)
	return val
}
