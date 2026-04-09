package api

import (
	"log"
	"net/http"
	"strings"

	"github.com/eduard256/jamperhub/internal/balancer"
	"github.com/eduard256/jamperhub/pkg/api"
	"github.com/eduard256/jamperhub/pkg/config"
	"github.com/eduard256/jamperhub/pkg/netutil"
)

func initNetwork() {
	api.HandleFunc("/api/network/interfaces", handleNetworkInterfaces)
	api.HandleFunc("/api/network/bridge", handleBridge)    // POST create
	api.HandleFunc("/api/network/bridge/", handleBridgeDel) // DELETE /api/network/bridge/{name}
	api.HandleFunc("/api/network", handleNetwork)
}

func handleNetworkInterfaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		api.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ifaces, err := netutil.ListInterfaces()
	if err != nil {
		api.Error(w, "network: "+err.Error(), http.StatusInternalServerError)
		return
	}
	api.Response(w, ifaces, http.StatusOK)
}

func handleNetwork(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		cfg := config.Get()
		api.Response(w, cfg.Network, http.StatusOK)

	case "PUT":
		var req config.Network
		if err := api.Decode(r, &req); err != nil {
			api.Error(w, "network: invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if err := config.SetNetwork(req); err != nil {
			api.Error(w, "network: save: "+err.Error(), http.StatusInternalServerError)
			return
		}

		log.Printf("[network] updated: input=%s output=%s subnet=%s", req.Input, req.Output, req.Subnet)
		balancer.Reload()
		api.OK(w, "Network config applied")

	default:
		api.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleBridge(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		api.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, "network: invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		req.Name = "br-jamperhub"
	}

	if err := netutil.CreateBridge(req.Name); err != nil {
		api.Error(w, "network: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("[network] bridge created: %s", req.Name)
	api.Response(w, map[string]any{
		"ok":      true,
		"name":    req.Name,
		"message": "Bridge created",
	}, http.StatusCreated)
}

func handleBridgeDel(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" {
		api.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/api/network/bridge/")
	if name == "" {
		api.Error(w, "network: bridge name required", http.StatusBadRequest)
		return
	}

	if err := netutil.DeleteBridge(name); err != nil {
		api.Error(w, "network: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("[network] bridge deleted: %s", name)
	api.OK(w, "Bridge "+name+" removed")
}
