package api

import (
	"log"
	"net/http"

	"github.com/eduard256/jamperhub/internal/balancer"
	"github.com/eduard256/jamperhub/pkg/api"
)

func initMigration() {
	api.HandleFunc("/api/migration", handleMigration)
	api.HandleFunc("/api/migration/cancel", handleMigrationCancel)
}

func handleMigration(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		api.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	api.Response(w, balancer.GetMigration(), http.StatusOK)
}

func handleMigrationCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		api.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	balancer.CancelMigration()
	log.Printf("[migration] cancelled by user")
	api.OK(w, "Migration cancelled")
}
