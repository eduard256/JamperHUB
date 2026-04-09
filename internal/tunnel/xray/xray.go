package xray

import (
	"path/filepath"
	"sync"

	"github.com/eduard256/jamperhub/pkg/config"
	"github.com/eduard256/jamperhub/pkg/netutil"
	"github.com/eduard256/jamperhub/pkg/tunnel"
	"github.com/eduard256/jamperhub/pkg/xray"
)

var (
	mu       sync.Mutex
	nextPort = 10808 // auto-increment SOCKS5 ports
)

func Init() {
	tunnel.HandleFunc("xray", tunnel.TypeInfo{
		Name:           "Xray (VLESS/VMess)",
		Mode:           "proxy",
		ConfigFormat:   "json",
		FileExtensions: []string{".json"},
		ConfigPlaceholder: `{
  "inbounds": [{
    "protocol": "socks",
    "listen": "127.0.0.1",
    "port": 10808,
    "settings": {"udp": true}
  }],
  "outbounds": [{
    "protocol": "vless",
    "settings": {
      "vnext": [{
        "address": "",
        "port": 443,
        "users": [{
          "id": "",
          "encryption": "none",
          "flow": "xtls-rprx-vision"
        }]
      }]
    },
    "streamSettings": {
      "network": "tcp",
      "security": "reality",
      "realitySettings": {
        "serverName": "",
        "fingerprint": "chrome",
        "publicKey": "",
        "shortId": ""
      }
    }
  }]
}`,
	}, factory)
}

func factory(id, configData string) tunnel.Client {
	binDir := filepath.Join(config.DataPath(), "bin")
	xrayBin := filepath.Join(binDir, "xray")
	tun2Bin := filepath.Join(binDir, "tun2socks")

	port := allocPort()

	client, err := xray.NewClient(id, xrayBin, tun2Bin, config.DataPath(), configData, port, netutil.FwMark())
	if err != nil {
		return &errClient{err: err}
	}
	return &wrapper{client: client}
}

func allocPort() int {
	mu.Lock()
	defer mu.Unlock()
	port := nextPort
	nextPort++
	return port
}

// wrapper adapts xray.Client to tunnel.Client interface
type wrapper struct {
	client *xray.Client
}

func (w *wrapper) Start() error      { return w.client.Start() }
func (w *wrapper) Stop() error       { return w.client.Stop() }
func (w *wrapper) Running() bool     { return w.client.Running() }
func (w *wrapper) Interface() string { return w.client.Interface() }
func (w *wrapper) TunName() string   { return w.client.TunName() }
func (w *wrapper) Mode() tunnel.Mode { return tunnel.ModeProxy }

// errClient is returned when config parsing fails
type errClient struct {
	err error
}

func (e *errClient) Start() error      { return e.err }
func (e *errClient) Stop() error       { return nil }
func (e *errClient) Running() bool     { return false }
func (e *errClient) Interface() string { return "" }
func (e *errClient) Mode() tunnel.Mode { return tunnel.ModeProxy }
