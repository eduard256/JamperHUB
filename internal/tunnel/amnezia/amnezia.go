package amnezia

import (
	"github.com/eduard256/jamperhub/pkg/tunnel"
)

func Init() {
	tunnel.HandleFunc("amnezia", tunnel.TypeInfo{
		Name:           "AmneziaWG (v1/v2)",
		Mode:           "tun",
		ConfigFormat:   "ini",
		FileExtensions: []string{".conf"},
		ConfigPlaceholder: `[Interface]
PrivateKey =
Address =
DNS = 1.1.1.1

[Peer]
PublicKey =
Endpoint = :51820
AllowedIPs = 0.0.0.0/0`,
	}, func(id, configData string) tunnel.Client {
		return &client{id: id, configData: configData}
	})
}

// client implements tunnel.Client for AmneziaWG
type client struct {
	id         string
	configData string
	running    bool
}

func (c *client) Start() error {
	// TODO: exec amneziawg-go, configure tun interface
	c.running = true
	return nil
}

func (c *client) Stop() error {
	// TODO: kill amneziawg-go process, remove tun
	c.running = false
	return nil
}

func (c *client) Running() bool { return c.running }
func (c *client) Interface() string { return "awg-" + c.id }
func (c *client) Mode() tunnel.Mode { return tunnel.ModeTUN }
