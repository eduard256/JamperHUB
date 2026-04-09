package api

import (
	"net/http"

	"github.com/eduard256/jamperhub/internal/balancer"
	"github.com/eduard256/jamperhub/pkg/api"
	"github.com/eduard256/jamperhub/pkg/config"
)

func initStatus() {
	api.HandleFunc("/api/status", handleStatus)
	api.HandleFunc("/api/status/tunnels", handleStatusTunnels)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		api.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfg := config.Get()
	state := balancer.GetState()

	result := map[string]any{
		"state":  state.SystemState,
		"uptime": state.Uptime,
		"first_run": config.IsFirstRun(),
		"network": map[string]any{
			"input_interface":  cfg.Network.Input,
			"input_ip":         state.InputIP,
			"input_gateway":    state.InputGateway,
			"internet":         state.HasInternet,
			"output_interface": cfg.Network.Output,
			"output_ip":        state.OutputIP,
			"output_mode":      cfg.Network.OutputMode,
		},
		"active_tunnel": state.ActiveTunnel,
		"tunnels_up":    state.TunnelsUp,
		"tunnels_down":  state.TunnelsDown,
		"tunnels_total": state.TunnelsTotal,
		"dhcp_leases":   state.DHCPLeases,
		"migration":     state.Migration,
	}

	api.Response(w, result, http.StatusOK)
}

func handleStatusTunnels(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		api.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	api.Response(w, balancer.GetTunnelStatuses(), http.StatusOK)
}
