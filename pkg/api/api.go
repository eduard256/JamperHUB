package api

import (
	"encoding/json"
	"net/http"
	"sync"
)

var mu sync.Mutex
var mux *http.ServeMux

func Init(m *http.ServeMux) {
	mux = m
}

// HandleFunc registers an API handler on the shared mux
func HandleFunc(pattern string, handler http.HandlerFunc) {
	mu.Lock()
	mux.HandleFunc(pattern, handler)
	mu.Unlock()
}

// Response sends a JSON response with the given status code
func Response(w http.ResponseWriter, v any, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// OK sends {"ok": true, "message": ...}
func OK(w http.ResponseWriter, msg string) {
	Response(w, map[string]any{"ok": true, "message": msg}, http.StatusOK)
}

// Error sends {"ok": false, "error": ...}
func Error(w http.ResponseWriter, err string, status int) {
	Response(w, map[string]any{"ok": false, "error": err}, status)
}

// Decode reads JSON body into v
func Decode(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}
