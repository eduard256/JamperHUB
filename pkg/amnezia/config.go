package amnezia

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// Config represents a parsed AmneziaWG .conf file
type Config struct {
	// Interface
	PrivateKey string
	Address    string
	DNS        string
	MTU        int
	ListenPort int

	// Amnezia obfuscation
	Jc   int
	Jmin int
	Jmax int
	S1   int
	S2   int
	S3   int
	S4   int
	H1   string
	H2   string
	H3   string
	H4   string
	I1   string
	I2   string
	I3   string
	I4   string
	I5   string

	// Peer
	PeerPublicKey string
	PeerPSK       string
	Endpoint      string
	AllowedIPs    []string
	PersistentKA  int
}

// ParseConfig parses an AmneziaWG .conf format config
func ParseConfig(data string) (*Config, error) {
	cfg := &Config{}
	section := ""

	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		lower := strings.ToLower(line)
		if lower == "[interface]" {
			section = "interface"
			continue
		}
		if lower == "[peer]" {
			section = "peer"
			continue
		}

		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)

		switch section {
		case "interface":
			parseInterface(cfg, key, val)
		case "peer":
			parsePeer(cfg, key, val)
		}
	}

	if cfg.PrivateKey == "" {
		return nil, fmt.Errorf("amnezia: missing PrivateKey")
	}
	if cfg.PeerPublicKey == "" {
		return nil, fmt.Errorf("amnezia: missing Peer PublicKey")
	}
	return cfg, nil
}

func parseInterface(cfg *Config, key, val string) {
	switch key {
	case "PrivateKey":
		cfg.PrivateKey = val
	case "Address":
		cfg.Address = val
	case "DNS":
		cfg.DNS = val
	case "MTU":
		fmt.Sscanf(val, "%d", &cfg.MTU)
	case "ListenPort":
		fmt.Sscanf(val, "%d", &cfg.ListenPort)
	case "Jc":
		fmt.Sscanf(val, "%d", &cfg.Jc)
	case "Jmin":
		fmt.Sscanf(val, "%d", &cfg.Jmin)
	case "Jmax":
		fmt.Sscanf(val, "%d", &cfg.Jmax)
	case "S1":
		fmt.Sscanf(val, "%d", &cfg.S1)
	case "S2":
		fmt.Sscanf(val, "%d", &cfg.S2)
	case "S3":
		fmt.Sscanf(val, "%d", &cfg.S3)
	case "S4":
		fmt.Sscanf(val, "%d", &cfg.S4)
	case "H1":
		cfg.H1 = val
	case "H2":
		cfg.H2 = val
	case "H3":
		cfg.H3 = val
	case "H4":
		cfg.H4 = val
	case "I1":
		cfg.I1 = val
	case "I2":
		cfg.I2 = val
	case "I3":
		cfg.I3 = val
	case "I4":
		cfg.I4 = val
	case "I5":
		cfg.I5 = val
	}
}

func parsePeer(cfg *Config, key, val string) {
	switch key {
	case "PublicKey":
		cfg.PeerPublicKey = val
	case "PresharedKey":
		cfg.PeerPSK = val
	case "Endpoint":
		cfg.Endpoint = val
	case "AllowedIPs":
		for _, ip := range strings.Split(val, ",") {
			ip = strings.TrimSpace(ip)
			if ip != "" {
				cfg.AllowedIPs = append(cfg.AllowedIPs, ip)
			}
		}
	case "PersistentKeepalive":
		fmt.Sscanf(val, "%d", &cfg.PersistentKA)
	}
}

// BuildUAPI builds a UAPI set config string from parsed config.
// Keys are hex-encoded, amnezia fields included.
func (c *Config) BuildUAPI() (string, error) {
	var sb strings.Builder

	privHex, err := b64toHex(c.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("amnezia: private_key: %w", err)
	}
	fmt.Fprintf(&sb, "private_key=%s\n", privHex)

	if c.ListenPort > 0 {
		fmt.Fprintf(&sb, "listen_port=%d\n", c.ListenPort)
	}

	sb.WriteString("replace_peers=true\n")

	// amnezia obfuscation fields
	writeInt(&sb, "jc", c.Jc)
	writeInt(&sb, "jmin", c.Jmin)
	writeInt(&sb, "jmax", c.Jmax)
	writeInt(&sb, "s1", c.S1)
	writeInt(&sb, "s2", c.S2)
	writeInt(&sb, "s3", c.S3)
	writeInt(&sb, "s4", c.S4)
	writeStr(&sb, "h1", c.H1)
	writeStr(&sb, "h2", c.H2)
	writeStr(&sb, "h3", c.H3)
	writeStr(&sb, "h4", c.H4)
	writeStr(&sb, "i1", c.I1)
	writeStr(&sb, "i2", c.I2)
	writeStr(&sb, "i3", c.I3)
	writeStr(&sb, "i4", c.I4)
	writeStr(&sb, "i5", c.I5)

	// peer
	pubHex, err := b64toHex(c.PeerPublicKey)
	if err != nil {
		return "", fmt.Errorf("amnezia: public_key: %w", err)
	}
	fmt.Fprintf(&sb, "public_key=%s\n", pubHex)

	if c.PeerPSK != "" {
		pskHex, err := b64toHex(c.PeerPSK)
		if err != nil {
			return "", fmt.Errorf("amnezia: preshared_key: %w", err)
		}
		fmt.Fprintf(&sb, "preshared_key=%s\n", pskHex)
	}

	if c.Endpoint != "" {
		fmt.Fprintf(&sb, "endpoint=%s\n", c.Endpoint)
	}
	if c.PersistentKA > 0 {
		fmt.Fprintf(&sb, "persistent_keepalive_interval=%d\n", c.PersistentKA)
	}

	sb.WriteString("replace_allowed_ips=true\n")
	for _, ip := range c.AllowedIPs {
		fmt.Fprintf(&sb, "allowed_ip=%s\n", ip)
	}

	return sb.String(), nil
}

// internals

func b64toHex(s string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	if len(raw) != 32 {
		return "", fmt.Errorf("key must be 32 bytes, got %d", len(raw))
	}
	return hex.EncodeToString(raw), nil
}

func writeInt(sb *strings.Builder, key string, val int) {
	if val > 0 {
		fmt.Fprintf(sb, "%s=%d\n", key, val)
	}
}

func writeStr(sb *strings.Builder, key, val string) {
	if val != "" {
		fmt.Fprintf(sb, "%s=%s\n", key, val)
	}
}
