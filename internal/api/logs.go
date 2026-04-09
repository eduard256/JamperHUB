package api

import (
	"net/http"
	"strconv"

	"github.com/eduard256/jamperhub/internal/store"
	"github.com/eduard256/jamperhub/pkg/api"
)

func initLogs() {
	api.HandleFunc("/api/logs", handleLogs)
}

func handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		api.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit := 50
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}

	typ := r.URL.Query().Get("type") // "all", "problem", "failover", etc.

	events := store.GetEvents(limit, typ)
	api.Response(w, events, http.StatusOK)
}
