package api

import (
	"net/http"
	"strings"

	"github.com/eduard256/jamperhub/internal/store"
	"github.com/eduard256/jamperhub/pkg/api"
)

func initMetrics() {
	api.HandleFunc("/api/metrics/summary", handleMetricsSummary)
	api.HandleFunc("/api/metrics/traffic", handleMetricsTraffic)
	api.HandleFunc("/api/metrics/traffic/clients", handleMetricsTrafficClients)
	api.HandleFunc("/api/metrics/tunnels", handleMetricsTunnels)
	api.HandleFunc("/api/metrics/tunnel/", handleMetricsTunnelDetail) // /api/metrics/tunnel/{id}/latency
	api.HandleFunc("/api/metrics/stability", handleMetricsStability)
	api.HandleFunc("/api/metrics/events", handleMetricsEvents)
}

func parsePeriod(r *http.Request) string {
	p := r.URL.Query().Get("period")
	switch p {
	case "1h", "24h", "7d", "30d":
		return p
	default:
		return "24h"
	}
}

func handleMetricsSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		api.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	api.Response(w, store.GetSummary(), http.StatusOK)
}

func handleMetricsTraffic(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		api.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	period := parsePeriod(r)
	api.Response(w, store.GetTrafficHistory(period), http.StatusOK)
}

func handleMetricsTrafficClients(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		api.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	api.Response(w, store.GetClientTraffic(), http.StatusOK)
}

func handleMetricsTunnels(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		api.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	period := parsePeriod(r)
	api.Response(w, store.GetTunnelMetrics(period), http.StatusOK)
}

func handleMetricsTunnelDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		api.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// /api/metrics/tunnel/{id}/latency
	path := strings.TrimPrefix(r.URL.Path, "/api/metrics/tunnel/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 {
		api.Error(w, "metrics: expected /api/metrics/tunnel/{id}/latency", http.StatusBadRequest)
		return
	}

	id := parts[0]
	metric := parts[1]
	period := parsePeriod(r)

	switch metric {
	case "latency":
		api.Response(w, store.GetTunnelLatencyHistory(id, period), http.StatusOK)
	default:
		api.Error(w, "metrics: unknown metric: "+metric, http.StatusBadRequest)
	}
}

func handleMetricsStability(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		api.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	period := parsePeriod(r)
	api.Response(w, store.GetStabilityTimeline(period), http.StatusOK)
}

func handleMetricsEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		api.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	period := parsePeriod(r)
	typ := r.URL.Query().Get("type")
	api.Response(w, store.GetMetricEvents(period, typ), http.StatusOK)
}
