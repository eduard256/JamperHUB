package api

import (
	"log"
	"net/http"

	"github.com/eduard256/jamperhub/internal/balancer"
	"github.com/eduard256/jamperhub/pkg/api"
	"github.com/eduard256/jamperhub/pkg/config"
)

func initBalancer() {
	api.HandleFunc("/api/balancer", handleBalancer)
}

func handleBalancer(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		cfg := config.Get()
		state := balancer.GetState()

		result := map[string]any{
			"config":         cfg.Balancer,
			"pending_switch": state.Migration,
		}
		api.Response(w, result, http.StatusOK)

	case "PUT":
		var req config.Balancer
		if err := api.Decode(r, &req); err != nil {
			api.Error(w, "balancer: invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if err := config.SetBalancer(req); err != nil {
			api.Error(w, "balancer: save: "+err.Error(), http.StatusInternalServerError)
			return
		}

		balancer.Reload()
		log.Printf("[balancer] updated config")
		api.OK(w, "Balancer config applied")

	default:
		api.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
