# API

All endpoints return JSON. Base URL: `http://YOUR_IP:7891`

## Status

### `GET /api/status`

System overview: active tunnel, network state, tunnel counts.

```json
{
  "state": "running",
  "uptime": 3600,
  "first_run": false,
  "network": {
    "input_interface": "enp6s18",
    "input_ip": "10.0.1.101/24",
    "input_gateway": "10.0.1.1",
    "internet": true,
    "output_interface": "enp6s19",
    "output_ip": "198.18.0.1/24",
    "output_mode": "physical"
  },
  "active_tunnel": {
    "id": "t-5be3c8339a55",
    "name": "Amsterdam Amnezia",
    "type": "amnezia",
    "latency": 190,
    "uptime": 3600,
    "speed_mbps": 57.8
  },
  "tunnels_up": 2,
  "tunnels_down": 0,
  "tunnels_total": 2,
  "dhcp_leases": 3,
  "migration": null
}
```

States: `running`, `waiting` (no internet), `no_tunnels`, `all_down`, `first_run`.

### `GET /api/status/tunnels`

Detailed status of each tunnel.

```json
[
  {
    "id": "t-5be3c8339a55",
    "name": "Amsterdam Amnezia",
    "type": "amnezia",
    "mode": "tun",
    "enabled": true,
    "state": "active",
    "latency": 190,
    "speed_mbps": 57.8,
    "uptime": 3600,
    "traffic_in": 10248460,
    "traffic_out": 128323,
    "interface": "awg-5be3c833",
    "last_speed_test": "2026-04-09T14:30:00Z"
  }
]
```

Tunnel states: `active`, `ready`, `connecting`, `down`, `disabled`.

## Config

### `GET /api/config`

Returns the full config.json.

### `PUT /api/config`

Replace the full config. Body = JSON config. Works from textarea or file upload.

```bash
curl -X PUT http://IP:7891/api/config -d @config.json
```

## Tunnels

### `GET /api/tunnels/types`

List of supported tunnel types. Used by UI for the "add tunnel" form.

```json
[
  {
    "type": "amnezia",
    "name": "AmneziaWG (v1/v2)",
    "mode": "tun",
    "config_format": "ini",
    "file_extensions": [".conf"]
  },
  {
    "type": "xray",
    "name": "Xray (VLESS/VMess)",
    "mode": "proxy",
    "config_format": "json",
    "file_extensions": [".json"]
  }
]
```

### `GET /api/tunnels`

List all configured tunnels.

### `POST /api/tunnels`

Add a new tunnel. No restart required.

```bash
curl -X POST http://IP:7891/api/tunnels \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "My VPN",
    "type": "amnezia",
    "enabled": true,
    "config_data": "[Interface]\nPrivateKey = ...\n\n[Peer]\n..."
  }'
```

### `GET /api/tunnels/{id}`

Get a single tunnel config.

### `PUT /api/tunnels/{id}`

Update tunnel. Partial update -- only send fields you want to change.

```bash
curl -X PUT http://IP:7891/api/tunnels/t-abc123 \
  -d '{"name": "New name", "enabled": false}'
```

### `DELETE /api/tunnels/{id}`

Stop and remove a tunnel.

### `POST /api/tunnels/{id}/restart`

Restart a tunnel.

### `POST /api/tunnels/{id}/activate`

Force a tunnel to become the active one.

## Network

### `GET /api/network/interfaces`

List all network interfaces on the machine.

### `GET /api/network`

Current network config (input, output, subnet).

### `PUT /api/network`

```bash
curl -X PUT http://IP:7891/api/network \
  -d '{"input":"enp6s18","output":"enp6s19","output_mode":"physical","subnet":"198.18.0.0/24"}'
```

### `POST /api/network/bridge`

Create a virtual bridge interface.

```bash
curl -X POST http://IP:7891/api/network/bridge -d '{"name":"br-jamperhub"}'
```

### `DELETE /api/network/bridge/{name}`

Remove a bridge.

## DHCP

### `GET /api/dhcp`

Config + current leases.

```json
{
  "config": {
    "enabled": true,
    "range_start": "198.18.0.100",
    "range_end": "198.18.0.200",
    "lease_time": "12h",
    "dns": ["8.8.8.8", "1.1.1.1"]
  },
  "leases": [
    {"ip": "198.18.0.101", "mac": "aa:bb:cc:dd:ee:01", "hostname": "iphone", "expires": "2026-04-10T08:00:00Z"}
  ]
}
```

### `PUT /api/dhcp`

Update DHCP config. Dnsmasq restarts automatically.

## Balancer

### `GET /api/balancer`

Balancer config + pending migration state.

### `PUT /api/balancer`

```bash
curl -X PUT http://IP:7891/api/balancer \
  -d '{
    "healthcheck_interval": 10,
    "speed_test_interval": 900,
    "test_url": "http://cp.cloudflare.com/generate_204",
    "fallback_direct": true,
    "switch_threshold_percent": 35,
    "switch_cooldown": 1800
  }'
```

## Migration

### `GET /api/migration`

Current migration state. Returns `null` if no migration in progress.

States: `evaluating`, `waiting_for_idle`, `switching`, `completed`, `cancelled`, `cooldown`.

### `POST /api/migration/cancel`

Cancel a pending speed-based migration.

## Logs

### `GET /api/logs?limit=50&type=all`

Event log. Types: `all`, `problem`, `failover`, `tunnel_up`, `tunnel_down`, `network`.

## Metrics

### `GET /api/metrics/summary`

Aggregate stats: total traffic, failover count.

### `GET /api/metrics/traffic?period=24h`

Traffic volume over time. Periods: `1h`, `24h`, `7d`, `30d`.

### `GET /api/metrics/traffic/clients`

Per-device traffic stats.

### `GET /api/metrics/tunnels?period=24h`

Per-tunnel metrics: avg/min/max latency, traffic.

### `GET /api/metrics/tunnel/{id}/latency?period=24h`

Latency history for a specific tunnel.

### `GET /api/metrics/tunnel/{id}/speed?period=24h`

Speed test history for a specific tunnel.

### `GET /api/metrics/stability?period=7d`

Timeline of system states (ok, failover, no_internet, fallback_direct).

### `GET /api/metrics/events?period=24h&type=problem`

Filtered events with severity.
