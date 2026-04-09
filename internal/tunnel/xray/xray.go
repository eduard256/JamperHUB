package xray

import (
	"github.com/eduard256/jamperhub/pkg/tunnel"
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
	}, func(id, configData string) tunnel.Client {
		return &client{id: id, configData: configData}
	})
}

// client implements tunnel.Client for Xray
type client struct {
	id         string
	configData string
	running    bool
	port       int // SOCKS5 listen port
}

func (c *client) Start() error {
	// TODO: write config to temp file, exec xray, start tun2socks
	c.running = true
	return nil
}

func (c *client) Stop() error {
	// TODO: kill xray + tun2socks processes
	c.running = false
	return nil
}

func (c *client) Running() bool { return c.running }

func (c *client) Interface() string {
	if c.port == 0 {
		return "socks5://127.0.0.1:10808"
	}
	return "socks5://127.0.0.1:" + itoa(c.port)
}

func (c *client) Mode() tunnel.Mode { return tunnel.ModeProxy }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	b := make([]byte, 0, 5)
	for n > 0 {
		b = append(b, byte('0'+n%10))
		n /= 10
	}
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}
