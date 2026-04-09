package api

import (
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/eduard256/jamperhub/pkg/api"
)

var listen string

func Init(addr string) {
	listen = addr
	mux := http.NewServeMux()
	api.Init(mux)

	// register all API routes
	initStatus()
	initConfig()
	initTunnels()
	initNetwork()
	initDHCP()
	initBalancer()
	initLogs()
	initMigration()
	initMetrics()

	// serve web UI static files
	mux.HandleFunc("/", handleStatic)

	go func() {
		log.Printf("[api] listen %s", listen)
		if err := http.ListenAndServe(listen, withCORS(mux)); err != nil {
			log.Fatalf("[api] %v", err)
		}
	}()
}

func withCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func handleStatic(w http.ResponseWriter, r *http.Request) {
	// API routes are handled by specific handlers
	// everything else is static files
	if strings.HasPrefix(r.URL.Path, "/api/") {
		api.Error(w, "not found", http.StatusNotFound)
		return
	}
	// TODO: serve embedded web/ files
	w.Header().Set("Content-Type", "text/html")
	io.WriteString(w, "<h1>JamperHUB</h1><p>Web UI coming soon</p>")
}
