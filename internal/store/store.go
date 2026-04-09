package store

import (
	"database/sql"
	"fmt"
	"log"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

const maxDBSize = 25 * 1024 * 1024 // 25 MB

func Init(dataPath string) error {
	dbPath := filepath.Join(dataPath, "data.db")

	var err error
	db, err = sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL")
	if err != nil {
		return fmt.Errorf("store: open: %w", err)
	}

	if err := migrate(); err != nil {
		return fmt.Errorf("store: migrate: %w", err)
	}

	go cleanupLoop()
	return nil
}

func migrate() error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			time DATETIME DEFAULT (datetime('now')),
			level TEXT NOT NULL,
			event TEXT NOT NULL,
			tunnel_id TEXT,
			message TEXT NOT NULL,
			duration INTEGER
		);
		CREATE INDEX IF NOT EXISTS idx_events_time ON events(time);
		CREATE INDEX IF NOT EXISTS idx_events_event ON events(event);

		CREATE TABLE IF NOT EXISTS traffic_points (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			time DATETIME DEFAULT (datetime('now')),
			tunnel_id TEXT NOT NULL,
			traffic_in INTEGER NOT NULL,
			traffic_out INTEGER NOT NULL,
			latency INTEGER,
			state TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_traffic_time ON traffic_points(time);
		CREATE INDEX IF NOT EXISTS idx_traffic_tunnel ON traffic_points(tunnel_id);

		CREATE TABLE IF NOT EXISTS client_traffic (
			ip TEXT PRIMARY KEY,
			mac TEXT,
			hostname TEXT,
			traffic_in INTEGER DEFAULT 0,
			traffic_out INTEGER DEFAULT 0,
			last_seen DATETIME
		);

		CREATE TABLE IF NOT EXISTS stability_intervals (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			time_from DATETIME NOT NULL,
			time_to DATETIME,
			state TEXT NOT NULL,
			active_tunnel TEXT,
			from_tunnel TEXT,
			to_tunnel TEXT,
			reason TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_stability_time ON stability_intervals(time_from);
	`)
	return err
}

// AddEvent stores a log event
func AddEvent(level, event, tunnelID, message string, duration int) {
	if db == nil {
		return
	}
	_, err := db.Exec(
		`INSERT INTO events (level, event, tunnel_id, message, duration) VALUES (?, ?, ?, ?, ?)`,
		level, event, nullStr(tunnelID), message, nullInt(duration),
	)
	if err != nil {
		log.Printf("[store] add event: %v", err)
	}
}

// GetEvents returns recent events, optionally filtered by type
func GetEvents(limit int, eventType string) []map[string]any {
	query := `SELECT time, level, event, tunnel_id, message, duration FROM events`
	var args []any

	if eventType != "" && eventType != "all" {
		query += ` WHERE event = ?`
		args = append(args, eventType)
	}
	query += ` ORDER BY time DESC LIMIT ?`
	args = append(args, limit)

	return queryMaps(query, args...)
}

// AddTrafficPoint records a healthcheck data point
func AddTrafficPoint(tunnelID string, trafficIn, trafficOut int64, latency *int, state string) {
	if db == nil {
		return
	}
	_, err := db.Exec(
		`INSERT INTO traffic_points (tunnel_id, traffic_in, traffic_out, latency, state) VALUES (?, ?, ?, ?, ?)`,
		tunnelID, trafficIn, trafficOut, latency, state,
	)
	if err != nil {
		log.Printf("[store] add traffic: %v", err)
	}
}

// GetSummary returns aggregate metrics
func GetSummary() map[string]any {
	// total traffic across all tunnels
	var totalIn, totalOut int64
	row := db.QueryRow(`SELECT COALESCE(SUM(traffic_in), 0), COALESCE(SUM(traffic_out), 0) FROM traffic_points`)
	row.Scan(&totalIn, &totalOut)

	// failover count
	var failovers int
	row = db.QueryRow(`SELECT COUNT(*) FROM events WHERE event = 'failover'`)
	row.Scan(&failovers)

	// internet downtime events
	var downtime int
	row = db.QueryRow(`SELECT COUNT(*) FROM events WHERE event = 'network_down'`)
	row.Scan(&downtime)

	return map[string]any{
		"total_traffic_in":  totalIn,
		"total_traffic_out": totalOut,
		"failover_count":    failovers,
		"network_down_count": downtime,
	}
}

// GetTrafficHistory returns traffic time series for graphs
func GetTrafficHistory(period string) map[string]any {
	interval, groupBy := periodToSQL(period)

	query := fmt.Sprintf(`
		SELECT %s as t, SUM(traffic_in) as ti, SUM(traffic_out) as tout
		FROM traffic_points
		WHERE time > datetime('now', ?)
		GROUP BY t ORDER BY t
	`, groupBy)

	return map[string]any{
		"period":   period,
		"interval": intervalSeconds(period),
		"points":   queryMaps(query, interval),
	}
}

// GetClientTraffic returns per-client traffic stats
func GetClientTraffic() []map[string]any {
	return queryMaps(`SELECT ip, mac, hostname, traffic_in, traffic_out, last_seen FROM client_traffic ORDER BY traffic_in DESC`)
}

// GetTunnelMetrics returns per-tunnel metrics for a period
func GetTunnelMetrics(period string) []map[string]any {
	interval, _ := periodToSQL(period)
	return queryMaps(`
		SELECT tunnel_id,
			SUM(traffic_in) as traffic_in, SUM(traffic_out) as traffic_out,
			AVG(latency) as avg_latency, MIN(latency) as min_latency, MAX(latency) as max_latency
		FROM traffic_points
		WHERE time > datetime('now', ?) AND latency IS NOT NULL
		GROUP BY tunnel_id
	`, interval)
}

// GetTunnelLatencyHistory returns latency time series for a specific tunnel
func GetTunnelLatencyHistory(tunnelID, period string) map[string]any {
	interval, groupBy := periodToSQL(period)
	query := fmt.Sprintf(`
		SELECT %s as t, AVG(latency) as latency, state
		FROM traffic_points
		WHERE tunnel_id = ? AND time > datetime('now', ?)
		GROUP BY t ORDER BY t
	`, groupBy)

	return map[string]any{
		"id":       tunnelID,
		"period":   period,
		"interval": intervalSeconds(period),
		"points":   queryMaps(query, tunnelID, interval),
	}
}

// GetStabilityTimeline returns state intervals for the stability graph
func GetStabilityTimeline(period string) map[string]any {
	interval, _ := periodToSQL(period)
	return map[string]any{
		"period": period,
		"intervals": queryMaps(
			`SELECT time_from, time_to, state, active_tunnel, from_tunnel, to_tunnel, reason
			FROM stability_intervals WHERE time_from > datetime('now', ?) ORDER BY time_from`,
			interval,
		),
	}
}

// GetMetricEvents returns events filtered by type for a period
func GetMetricEvents(period, eventType string) []map[string]any {
	interval, _ := periodToSQL(period)
	query := `SELECT time, event AS type, level AS severity, tunnel_id, message, duration
		FROM events WHERE time > datetime('now', ?)`
	args := []any{interval}

	if eventType != "" && eventType != "all" {
		query += ` AND event = ?`
		args = append(args, eventType)
	}
	query += ` ORDER BY time DESC`

	return queryMaps(query, args...)
}

// internals

func cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	for range ticker.C {
		cleanup()
	}
}

func cleanup() {
	if db == nil {
		return
	}

	// check db file size
	var pageCount, pageSize int64
	db.QueryRow(`PRAGMA page_count`).Scan(&pageCount)
	db.QueryRow(`PRAGMA page_size`).Scan(&pageSize)
	dbSize := pageCount * pageSize

	if dbSize > maxDBSize {
		db.Exec(`DELETE FROM traffic_points WHERE time < datetime('now', '-7 days')`)
		db.Exec(`DELETE FROM events WHERE time < datetime('now', '-14 days')`)
		db.Exec(`VACUUM`)
		log.Printf("[store] cleanup: db was %d MB, exceeded limit %d MB", dbSize/1024/1024, maxDBSize/1024/1024)
		return
	}

	// routine cleanup
	db.Exec(`DELETE FROM traffic_points WHERE time < datetime('now', '-30 days')`)
	db.Exec(`DELETE FROM events WHERE time < datetime('now', '-30 days')`)
	db.Exec(`DELETE FROM stability_intervals WHERE time_from < datetime('now', '-30 days')`)
}

func periodToSQL(period string) (string, string) {
	switch period {
	case "1h":
		return "-1 hour", "strftime('%Y-%m-%d %H:%M', time, 'start of minute')"
	case "24h":
		return "-1 day", "strftime('%Y-%m-%d %H:%M', time, 'start of minute', printf('-%d minutes', CAST(strftime('%M', time) AS INTEGER) % 5))"
	case "7d":
		return "-7 days", "strftime('%Y-%m-%d %H:00', time)"
	case "30d":
		return "-30 days", "strftime('%Y-%m-%d %H:00', time, printf('-%d hours', CAST(strftime('%H', time) AS INTEGER) % 6))"
	default:
		return "-1 day", "strftime('%Y-%m-%d %H:%M', time)"
	}
}

func intervalSeconds(period string) int {
	switch period {
	case "1h":
		return 60
	case "24h":
		return 300
	case "7d":
		return 3600
	case "30d":
		return 21600
	default:
		return 300
	}
}

func queryMaps(query string, args ...any) []map[string]any {
	if db == nil {
		return []map[string]any{}
	}
	rows, err := db.Query(query, args...)
	if err != nil {
		log.Printf("[store] query: %v", err)
		return []map[string]any{}
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	var result []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		rows.Scan(ptrs...)
		row := map[string]any{}
		for i, col := range cols {
			row[col] = vals[i]
		}
		result = append(result, row)
	}
	if result == nil {
		return []map[string]any{}
	}
	return result
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullInt(n int) any {
	if n == 0 {
		return nil
	}
	return n
}
