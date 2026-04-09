package api

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/eduard256/jamperhub/internal/balancer"
	"github.com/eduard256/jamperhub/pkg/api"
	"github.com/eduard256/jamperhub/pkg/config"
)

func initConfig() {
	api.HandleFunc("/api/config", handleConfig)
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		api.Response(w, config.Get(), http.StatusOK)

	case "PUT":
		body, err := io.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			api.Error(w, "config: read body: "+err.Error(), http.StatusBadRequest)
			return
		}

		var cfg config.Config
		if err := json.Unmarshal(body, &cfg); err != nil {
			api.Error(w, "config: invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if err := config.Set(cfg); err != nil {
			api.Error(w, "config: save: "+err.Error(), http.StatusInternalServerError)
			return
		}

		log.Printf("[config] replaced via API")
		balancer.Reload()
		api.OK(w, "Config applied")

	default:
		api.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
