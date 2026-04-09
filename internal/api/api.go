package api

import (
	"io/fs"
	"log"
	"net/http"
	"strings"

	pkgapi "github.com/eduard256/jamperhub/pkg/api"
	"github.com/eduard256/jamperhub/web"
)

var listen string

func Init(addr string) {
	listen = addr
	mux := http.NewServeMux()
	pkgapi.Init(mux)

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

	// serve embedded web UI
	staticFS, _ := fs.Sub(web.Files, ".")
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			pkgapi.Error(w, "not found", http.StatusNotFound)
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		data, err := fs.ReadFile(staticFS, path)
		if err != nil {
			// fallback to index.html
			data, err = fs.ReadFile(staticFS, "index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			path = "index.html"
		}

		// content type
		ct := "application/octet-stream"
		if strings.HasSuffix(path, ".html") {
			ct = "text/html; charset=utf-8"
		} else if strings.HasSuffix(path, ".css") {
			ct = "text/css; charset=utf-8"
		} else if strings.HasSuffix(path, ".js") {
			ct = "application/javascript; charset=utf-8"
		} else if strings.HasSuffix(path, ".woff2") {
			ct = "font/woff2"
		} else if strings.HasSuffix(path, ".json") {
			ct = "application/json"
		}

		w.Header().Set("Content-Type", ct)
		w.Write(data)
	})

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
