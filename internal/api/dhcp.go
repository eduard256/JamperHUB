package api

import (
	"log"
	"net/http"

	"github.com/eduard256/jamperhub/internal/dhcp"
	"github.com/eduard256/jamperhub/pkg/api"
	"github.com/eduard256/jamperhub/pkg/config"
)

func initDHCP() {
	api.HandleFunc("/api/dhcp", handleDHCP)
}

func handleDHCP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		cfg := config.Get()
		leases := dhcp.GetLeases()
		api.Response(w, map[string]any{
			"config": cfg.DHCP,
			"leases": leases,
		}, http.StatusOK)

	case "PUT":
		var req config.DHCP
		if err := api.Decode(r, &req); err != nil {
			api.Error(w, "dhcp: invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if err := config.SetDHCP(req); err != nil {
			api.Error(w, "dhcp: save: "+err.Error(), http.StatusInternalServerError)
			return
		}

		dhcp.Reload()
		log.Printf("[dhcp] updated config")
		api.OK(w, "DHCP config applied, dnsmasq restarted")

	default:
		api.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
