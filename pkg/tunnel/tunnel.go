package tunnel

import "time"

// Mode defines how traffic is routed through the tunnel
type Mode int

const (
	ModeTUN   Mode = iota // tun interface, route via ip route
	ModeProxy             // SOCKS5/HTTP proxy, route via tun2socks
)

// State represents the current state of a tunnel
type State string

const (
	StateDisabled    State = "disabled"
	StateConnecting  State = "connecting"
	StateConnected   State = "connected"
	StateActive      State = "active"
	StateReady       State = "ready"
	StateDown        State = "down"
	StateUnhealthy   State = "unhealthy"
	StateRestarting  State = "restarting"
)

// Client is the universal interface for any tunnel: VPN, proxy, SSH, etc.
type Client interface {
	Start() error
	Stop() error
	Running() bool
	Interface() string // tun name for TUN mode, "socks5://127.0.0.1:PORT" for proxy mode
	Mode() Mode
}

// TunNamer is implemented by proxy-type clients that have a separate tun interface for routing
type TunNamer interface {
	TunName() string
}

// RoutingInterface returns the tun interface name to use for ip route.
// For TUN-type clients, this is Interface().
// For proxy-type clients with tun2socks, this is TunName().
func RoutingInterface(c Client) string {
	if tn, ok := c.(TunNamer); ok {
		return tn.TunName()
	}
	return c.Interface()
}

// Factory creates a Client from config data
type Factory func(id, configData string) Client

// Status holds runtime metrics for a tunnel
type Status struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	Type              string  `json:"type"`
	Mode              string  `json:"mode"` // "tun" or "proxy"
	Enabled           bool    `json:"enabled"`
	State             State   `json:"state"`
	Latency           *int    `json:"latency"`            // ms, nil = unknown
	SpeedMbps         *float64 `json:"speed_mbps"`        // nil = not tested yet
	Uptime            int64   `json:"uptime"`             // seconds
	TrafficIn         int64   `json:"traffic_in"`         // bytes
	TrafficOut        int64   `json:"traffic_out"`        // bytes
	ActiveConnections int     `json:"active_connections"`
	Interface         string  `json:"interface"`
	Error             *string `json:"error"`
	LastSpeedTest     *time.Time `json:"last_speed_test"`
}

// TypeInfo describes a registered tunnel type for the UI
type TypeInfo struct {
	Type              string   `json:"type"`
	Name              string   `json:"name"`
	Mode              string   `json:"mode"` // "tun" or "proxy"
	ConfigFormat      string   `json:"config_format"`
	ConfigPlaceholder string   `json:"config_placeholder"`
	FileExtensions    []string `json:"file_extensions"`
}

// registry of tunnel factories
var handlers = map[string]Factory{}

// registry of type info
var types = map[string]TypeInfo{}

// HandleFunc registers a tunnel factory for a given type
func HandleFunc(typ string, info TypeInfo, fn Factory) {
	handlers[typ] = fn
	info.Type = typ
	types[typ] = info
}

// NewClient creates a tunnel client by type
func NewClient(typ, id, configData string) Client {
	fn := handlers[typ]
	if fn == nil {
		return nil
	}
	return fn(id, configData)
}

// Types returns all registered tunnel types
func Types() []TypeInfo {
	result := make([]TypeInfo, 0, len(types))
	for _, t := range types {
		result = append(result, t)
	}
	return result
}

// HasType checks if a tunnel type is registered
func HasType(typ string) bool {
	_, ok := handlers[typ]
	return ok
}
