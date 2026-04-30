# JamperHUB

Fault-tolerant VPN gateway with automatic channel balancer. One binary, multiple VPN tunnels, instant failover.

![Dashboard](https://github.com/eduard256/JamperHUB/releases/download/v0.0.1/1.webp)

![Settings](https://github.com/eduard256/JamperHUB/releases/download/v0.0.1/2.webp)

![Latency](https://github.com/eduard256/JamperHUB/releases/download/v0.0.1/3.webp)

## How it works

1. Takes internet from one network interface
2. Connects to multiple VPN servers simultaneously
3. Monitors latency and speed, picks the fastest
4. If active tunnel dies -- switches to the next one instantly
5. Serves a local network with DHCP on the output interface
6. All traffic from connected devices goes through VPN

Supported protocols: **AmneziaWG** (v1/v2), **Xray** (VLESS/VMess/Reality). More coming.

## Priority System

Each tunnel has a priority level that controls when it's used and whether it stays connected.

**Priority 1 -- Primary.** Your own VPN server. Always connected, always active -- even if it's slower than the rest. The system only switches away from it when it goes down. As soon as it recovers, traffic moves back. If you have multiple priority 1 servers, the fastest one is used. Speed-based migration works within the same priority level.

**Priority 2 -- Recommended.** The default for most servers. Always connected, full monitoring, speed tests. Used when all priority 1 servers are down. Between each other, selected by speed. This is what you should use for most VPN configs.

**Priority 3 -- Backup.** Not connected at startup. Sits in standby, doesn't consume connections or resources. Only activates when every priority 1 and 2 server is down. Launched in pairs -- first one that connects gets the traffic. No speed optimization, just "get internet working". When a higher-priority server recovers, backup tunnels shut down automatically.

Priority can be changed at any time through the web UI or API -- no restart needed. The tunnel starts or stops immediately.

## Install

Download the binary:

```bash
# amd64
wget https://github.com/eduard256/JamperHUB/releases/download/v0.0.1/jamperhub-linux-amd64 -O jamperhub
chmod +x jamperhub

# arm64
wget https://github.com/eduard256/JamperHUB/releases/download/v0.0.1/jamperhub-linux-arm64 -O jamperhub
chmod +x jamperhub
```

Install dnsmasq (the only dependency):

```bash
sudo apt install dnsmasq
sudo systemctl disable dnsmasq
```

Run:

```bash
sudo ./jamperhub
```

Open `http://YOUR_IP:7891` in browser. Setup wizard will guide you through the initial configuration.

### Systemd

```bash
sudo mv jamperhub /usr/local/bin/
sudo tee /etc/systemd/system/jamperhub.service > /dev/null << 'EOF'
[Unit]
Description=JamperHUB VPN Gateway
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/jamperhub -data /etc/jamperhub -listen :7891
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now jamperhub
```

### Options

```
-data /etc/jamperhub    path to config and data directory
-listen :7891           HTTP listen address
-version                show version
```

## Data

All data is stored in the `-data` directory:

```
/etc/jamperhub/
  config.json       configuration (portable, copy to another machine)
  data.db           metrics and logs (SQLite, expendable)
  bin/              VPN client binaries (extracted on first run)
```

VPN client configs are stored inline in `config.json`. Copy one file -- everything works on a new machine.

## API

Full API documentation: [docs/api.md](docs/api.md)

## License

MIT
