package amnezia

import (
	"path/filepath"

	"github.com/eduard256/jamperhub/pkg/amnezia"
	"github.com/eduard256/jamperhub/pkg/config"
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
	}, factory)
}

func factory(id, configData string) tunnel.Client {
	binDir := filepath.Join(config.DataPath(), "bin")
	binPath := filepath.Join(binDir, "amneziawg-go")

	client, err := amnezia.NewClient(id, binPath, configData)
	if err != nil {
		// return a dead client that reports the error
		return &errClient{err: err}
	}
	return &wrapper{client: client}
}

// wrapper adapts amnezia.Client to tunnel.Client interface
type wrapper struct {
	client *amnezia.Client
}

func (w *wrapper) Start() error       { return w.client.Start() }
func (w *wrapper) Stop() error        { return w.client.Stop() }
func (w *wrapper) Running() bool      { return w.client.Running() }
func (w *wrapper) Interface() string  { return w.client.Interface() }
func (w *wrapper) Mode() tunnel.Mode  { return tunnel.ModeTUN }

// errClient is returned when config parsing fails
type errClient struct {
	err error
}

func (e *errClient) Start() error       { return e.err }
func (e *errClient) Stop() error        { return nil }
func (e *errClient) Running() bool      { return false }
func (e *errClient) Interface() string  { return "" }
func (e *errClient) Mode() tunnel.Mode  { return tunnel.ModeTUN }
