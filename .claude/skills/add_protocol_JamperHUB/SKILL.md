---
name: add_protocol_JamperHUB
description: Add a new tunnel protocol (VPN, proxy, or any other) to JamperHUB. Use when the user asks to add support for OpenVPN, WireGuard, Shadowsocks, SOCKS5, SSH tunnel, Tor, or any other protocol. Argument is the protocol name.
argument-hint: "[protocol-name]"
---

# Add New Protocol to JamperHUB

You are adding a new tunnel protocol to JamperHUB -- a VPN gateway with automatic failover.
The protocol name is provided as the argument to this skill.

## BEFORE YOU START

### Step 0: Research the protocol

Launch an agent to research the protocol on the internet. You need to know:

1. **Binary**: what binary to run, where to download (GitHub releases, package manager)
2. **Config format**: what config file format the protocol uses, full example
3. **How to start**: command-line arguments, foreground mode flag
4. **How to stop**: SIGTERM? SIGKILL? Special command?
5. **Network interface**: does it create a TUN/TAP device? Or expose a SOCKS5/HTTP proxy?
6. **Mode**: TUN (creates network interface, route traffic via `ip route`) or Proxy (SOCKS5/HTTP, needs tun2socks)
7. **Healthcheck**: how to verify the connection works
8. **Dependencies**: any system packages needed?

This is critical. Don't start coding until you have this information.

## REFERENCE FILES -- READ THESE FIRST

Two working implementations exist. Read them as examples before writing any code.

### TUN-type example: Amnezia (creates tun interface, traffic routed via ip route)

```
pkg/amnezia/config.go    -- parse .conf format, build UAPI config string
pkg/amnezia/uapi.go      -- UAPI socket protocol (connect, set, get)
pkg/amnezia/client.go    -- Start/Stop/Running/Interface, process lifecycle

internal/tunnel/amnezia/amnezia.go -- Init(), HandleFunc registration, factory
```

### Proxy-type example: Xray (exposes SOCKS5, tun2socks creates tun for routing)

```
pkg/xray/client.go       -- Start/Stop/Running/Interface/TunName, xray + tun2socks processes

internal/tunnel/xray/xray.go -- Init(), HandleFunc registration, factory, port allocation
```

### Key interfaces (read first):

```
pkg/tunnel/tunnel.go     -- Client interface, TunNamer, Mode, Factory, TypeInfo, registry
```

## ARCHITECTURE RULES

1. **pkg/{protocol}/** -- pure logic. Config parsing, process management, protocol-specific code. NEVER imports internal/.
2. **internal/tunnel/{protocol}/** -- glue only. Init(), HandleFunc registration, factory function. Imports pkg/{protocol}/.
3. **One line in cmd/jamperhub/main.go** -- import and Init() call.
4. **No changes to balancer, API, healthcheck, store** -- they work with tunnel.Client interface, protocol-agnostic.

## STEP-BY-STEP IMPLEMENTATION

### Step 1: Create pkg/{protocol}/client.go

This is the main file. It manages the process lifecycle.

Must implement `tunnel.Client` interface:

```go
type Client interface {
    Start() error       // launch the process, configure networking
    Stop() error        // kill the process, cleanup
    Running() bool      // is the process alive
    Interface() string  // TUN mode: tun interface name. Proxy mode: "socks5://127.0.0.1:PORT"
    Mode() Mode         // ModeTUN or ModeProxy
}
```

If Mode is **ModeProxy**, also implement `tunnel.TunNamer`:

```go
type TunNamer interface {
    TunName() string    // the tun2socks interface name for routing
}
```

**For TUN-type protocols** (like Amnezia, WireGuard, OpenVPN with tun):
- Start(): exec binary, configure interface (IP, bring up)
- Stop(): kill process, `ip link del <iface>`
- Interface(): return tun interface name (e.g. "wg-abc123")
- Mode(): return tunnel.ModeTUN

**For Proxy-type protocols** (like Xray, Shadowsocks, SOCKS5):
- Start(): exec binary + exec tun2socks with fwmark
- Stop(): kill both processes, `ip link del <tun-iface>`
- Interface(): return "socks5://127.0.0.1:PORT" (for healthcheck)
- TunName(): return tun2socks interface name (for routing)
- Mode(): return tunnel.ModeProxy

**Important details from existing code:**

- Interface names must be <= 15 chars (Linux limit). Use `shortID()` to truncate.
- For proxy types, tun2socks needs `-fwmark` flag. Get it from `netutil.FwMark()`.
- Wait for socket/port/interface to appear before proceeding (with timeout).
- Watch process in background goroutine, set running=false on exit.
- Use `sync.Mutex` for thread safety.

### Step 2: Create config parser (if needed)

If the protocol has its own config format (like Amnezia's .conf INI format), create `pkg/{protocol}/config.go`.

If the protocol uses JSON config (like Xray), you can work with it directly in client.go.

The config parser:
- Takes raw config string from `config_data` field
- Parses into a struct
- Validates required fields
- May transform the config (e.g. Xray: patch inbound port)

### Step 3: Create internal/tunnel/{protocol}/{protocol}.go

This is the glue. ~30-50 lines max. Pattern:

```go
package {protocol}

import (
    "path/filepath"

    "github.com/eduard256/jamperhub/pkg/{protocol}"
    "github.com/eduard256/jamperhub/pkg/config"
    "github.com/eduard256/jamperhub/pkg/tunnel"
)

func Init() {
    tunnel.HandleFunc("{protocol}", tunnel.TypeInfo{
        Name:              "{Protocol Display Name}",
        Mode:              "tun",  // or "proxy"
        ConfigFormat:      "ini",  // or "json", "yaml", "toml"
        FileExtensions:    []string{".conf"},
        ConfigPlaceholder: `...`,  // template shown in UI "add tunnel" form
    }, factory)
}

func factory(id, configData string) tunnel.Client {
    binDir := filepath.Join(config.DataPath(), "bin")
    // create and return client
    // on error, return &errClient{err: err}
}

// wrapper adapts {protocol}.Client to tunnel.Client
type wrapper struct { client *{protocol}.Client }
func (w *wrapper) Start() error       { return w.client.Start() }
func (w *wrapper) Stop() error        { return w.client.Stop() }
func (w *wrapper) Running() bool      { return w.client.Running() }
func (w *wrapper) Interface() string  { return w.client.Interface() }
func (w *wrapper) Mode() tunnel.Mode  { return tunnel.ModeTUN }

// errClient for config parsing failures
type errClient struct { err error }
func (e *errClient) Start() error       { return e.err }
func (e *errClient) Stop() error        { return nil }
func (e *errClient) Running() bool      { return false }
func (e *errClient) Interface() string  { return "" }
func (e *errClient) Mode() tunnel.Mode  { return tunnel.ModeTUN }
```

### Step 4: Register in main.go

File: `cmd/jamperhub/main.go`

Add import:
```go
"github.com/eduard256/jamperhub/internal/tunnel/{protocol}"
```

Add Init() call after existing tunnel registrations:
```go
// Step 4. Register tunnel types
amnezia.Init()
xray.Init()
{protocol}.Init()  // <-- add this line
```

### Step 5: Embed binary (if needed)

If the protocol needs a separate binary (like amneziawg-go, xray):

1. Download the binary for linux/amd64 and place in `pkg/bindata/amd64/{binary}.gz` (gzip -9)
2. Add entry to `pkg/bindata/bindata.go`:

```go
var bins = map[string]string{
    "amneziawg-go": "amd64/amneziawg-go.gz",
    "xray":         "amd64/xray.gz",
    "tun2socks":    "amd64/tun2socks.gz",
    "{binary}":     "amd64/{binary}.gz",  // <-- add this
}
```

3. Update `.github/workflows/release.yml` to download this binary in CI.

If the protocol binary is available via `apt install` (like openvpn), you can skip embedding and check at runtime with `exec.LookPath()`.

### Step 6: Build and test

```bash
go build ./cmd/jamperhub/
```

Test on the server:
1. Deploy new binary
2. Add tunnel via API:
```bash
curl -X POST http://IP:7891/api/tunnels \
  -H 'Content-Type: application/json' \
  -d '{"name":"Test","type":"{protocol}","enabled":true,"config_data":"..."}'
```
3. Check `/api/status/tunnels` -- should show the new tunnel
4. Wait for healthcheck -- should show latency
5. Verify traffic goes through the tunnel

## FILES TO UPDATE (CHECKLIST)

When adding a new protocol, these are ALL the files you touch:

| File | Action |
|------|--------|
| `pkg/{protocol}/client.go` | **CREATE** -- process lifecycle |
| `pkg/{protocol}/config.go` | **CREATE** (optional) -- config parser |
| `internal/tunnel/{protocol}/{protocol}.go` | **CREATE** -- Init, factory, wrapper |
| `cmd/jamperhub/main.go` | **EDIT** -- add import + Init() call |
| `pkg/bindata/bindata.go` | **EDIT** (if embedding binary) -- add to bins map |
| `pkg/bindata/amd64/{binary}.gz` | **CREATE** (if embedding) -- compressed binary |
| `.github/workflows/release.yml` | **EDIT** (if embedding) -- download in CI |

Files you do NOT touch:
- `pkg/tunnel/tunnel.go` -- interface is already universal
- `internal/balancer/` -- works with tunnel.Client, protocol-agnostic
- `internal/api/` -- works with tunnel registry, auto-discovers new types
- `pkg/healthcheck/` -- uses curl, works with any tun or socks5
- `internal/store/` -- stores metrics by tunnel ID, protocol-agnostic
- `web/` -- UI reads types from `/api/tunnels/types`, auto-shows new protocols

## PROXY-TYPE SPECIFIC NOTES

If the protocol exposes a SOCKS5 or HTTP proxy (not a TUN interface):

1. You need tun2socks to create a TUN from the proxy. It is already embedded.
2. Allocate unique SOCKS5 ports per instance (see `internal/tunnel/xray/xray.go` -- `nextPort` pattern).
3. Start tun2socks with `-fwmark` from `netutil.FwMark()` to prevent routing loops.
4. `Interface()` returns `"socks5://127.0.0.1:PORT"` -- used by healthcheck (curl -x).
5. `TunName()` returns the tun2socks interface name -- used by balancer for `ip route`.
6. Implement `tunnel.TunNamer` interface on the wrapper.

## NAMING CONVENTIONS

- Package name: lowercase, no hyphens (e.g. `openvpn`, `shadowsocks`, `sshtunnel`)
- Interface name: `{prefix}-{shortID}` where prefix is 3-4 chars, total <= 15 chars
  - TUN: `ovpn-abc12345` (openvpn), `ss-abc12345` (shadowsocks)
  - Proxy tun2socks: `tun-jh-abc12345`
- Binary name in embed: match upstream (e.g. `openvpn`, `ss-local`)
