package api

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"strings"

	"github.com/eduard256/jamperhub/internal/balancer"
	"github.com/eduard256/jamperhub/pkg/api"
	"github.com/eduard256/jamperhub/pkg/config"
	"github.com/eduard256/jamperhub/pkg/tunnel"
)

func initTunnels() {
	api.HandleFunc("/api/tunnels/types", handleTunnelTypes)
	api.HandleFunc("/api/tunnels", handleTunnels)
	api.HandleFunc("/api/tunnels/", handleTunnel) // /api/tunnels/{id}[/action]
}

func handleTunnelTypes(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		api.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	api.Response(w, tunnel.Types(), http.StatusOK)
}

func handleTunnels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		cfg := config.Get()
		api.Response(w, cfg.Servers, http.StatusOK)

	case "POST":
		var req struct {
			Name       string `json:"name"`
			Type       string `json:"type"`
			Enabled    bool   `json:"enabled"`
			ConfigData string `json:"config_data"`
		}
		if err := api.Decode(r, &req); err != nil {
			api.Error(w, "tunnels: invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Name == "" {
			api.Error(w, "tunnels: name is required", http.StatusBadRequest)
			return
		}
		if !tunnel.HasType(req.Type) {
			api.Error(w, "tunnels: unknown type: "+req.Type, http.StatusBadRequest)
			return
		}
		if req.ConfigData == "" {
			api.Error(w, "tunnels: config_data is required", http.StatusBadRequest)
			return
		}

		s := config.Server{
			ID:         genID(),
			Name:       req.Name,
			Type:       req.Type,
			Enabled:    req.Enabled,
			ConfigData: req.ConfigData,
		}
		if err := config.AddServer(s); err != nil {
			api.Error(w, "tunnels: save: "+err.Error(), http.StatusInternalServerError)
			return
		}

		log.Printf("[tunnels] added %s (%s, %s)", s.ID, s.Name, s.Type)
		if s.Enabled {
			balancer.StartTunnel(s.ID)
		}
		api.Response(w, s, http.StatusCreated)

	default:
		api.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleTunnel(w http.ResponseWriter, r *http.Request) {
	// parse /api/tunnels/{id} or /api/tunnels/{id}/action
	path := strings.TrimPrefix(r.URL.Path, "/api/tunnels/")
	parts := strings.SplitN(path, "/", 2)
	id := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	if id == "" {
		api.Error(w, "tunnels: id required", http.StatusBadRequest)
		return
	}

	// handle actions: /api/tunnels/{id}/restart, /api/tunnels/{id}/activate
	if action != "" {
		handleTunnelAction(w, r, id, action)
		return
	}

	switch r.Method {
	case "GET":
		s, ok := config.GetServer(id)
		if !ok {
			api.Error(w, "tunnels: not found: "+id, http.StatusNotFound)
			return
		}
		api.Response(w, s, http.StatusOK)

	case "PUT":
		var req struct {
			Name       *string `json:"name"`
			Enabled    *bool   `json:"enabled"`
			ConfigData *string `json:"config_data"`
		}
		if err := api.Decode(r, &req); err != nil {
			api.Error(w, "tunnels: invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		err := config.UpdateServer(id, func(s *config.Server) {
			if req.Name != nil {
				s.Name = *req.Name
			}
			if req.Enabled != nil {
				s.Enabled = *req.Enabled
			}
			if req.ConfigData != nil {
				s.ConfigData = *req.ConfigData
			}
		})
		if err != nil {
			api.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		log.Printf("[tunnels] updated %s", id)

		// restart tunnel if config or enabled state changed
		if req.ConfigData != nil || req.Enabled != nil {
			balancer.RestartTunnel(id)
		}

		s, _ := config.GetServer(id)
		api.Response(w, s, http.StatusOK)

	case "DELETE":
		if _, ok := config.GetServer(id); !ok {
			api.Error(w, "tunnels: not found: "+id, http.StatusNotFound)
			return
		}

		balancer.StopTunnel(id)

		if err := config.RemoveServer(id); err != nil {
			api.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		log.Printf("[tunnels] deleted %s", id)
		api.OK(w, "Tunnel "+id+" stopped and removed")

	default:
		api.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleTunnelAction(w http.ResponseWriter, r *http.Request, id, action string) {
	if r.Method != "POST" {
		api.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if _, ok := config.GetServer(id); !ok {
		api.Error(w, "tunnels: not found: "+id, http.StatusNotFound)
		return
	}

	switch action {
	case "restart":
		balancer.RestartTunnel(id)
		log.Printf("[tunnels] restarted %s", id)
		api.OK(w, "Tunnel "+id+" restarted")

	case "activate":
		balancer.ActivateTunnel(id)
		log.Printf("[tunnels] activated %s", id)
		api.OK(w, "Tunnel "+id+" set as active")

	default:
		api.Error(w, "tunnels: unknown action: "+action, http.StatusBadRequest)
	}
}

func genID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return "t-" + hex.EncodeToString(b)
}
