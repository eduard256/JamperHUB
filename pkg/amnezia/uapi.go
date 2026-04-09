package amnezia

import (
	"bufio"
	"fmt"
	"net"
	"strings"
)

const socketDir = "/var/run/amneziawg"

// UAPI wraps a connection to the amneziawg-go UAPI socket
type UAPI struct {
	conn net.Conn
	rw   *bufio.ReadWriter
}

// Dial connects to the UAPI socket for the given interface
func Dial(ifaceName string) (*UAPI, error) {
	path := fmt.Sprintf("%s/%s.sock", socketDir, ifaceName)
	conn, err := net.Dial("unix", path)
	if err != nil {
		return nil, fmt.Errorf("amnezia: dial %s: %w", path, err)
	}
	return &UAPI{
		conn: conn,
		rw: bufio.NewReadWriter(
			bufio.NewReader(conn),
			bufio.NewWriter(conn),
		),
	}, nil
}

func (u *UAPI) Close() error { return u.conn.Close() }

// Set sends a UAPI set command with the given config string
func (u *UAPI) Set(config string) error {
	u.rw.WriteString("set=1\n")
	u.rw.WriteString(config)
	if !strings.HasSuffix(config, "\n") {
		u.rw.WriteString("\n")
	}
	u.rw.WriteString("\n")
	if err := u.rw.Flush(); err != nil {
		return fmt.Errorf("amnezia: flush set: %w", err)
	}
	return u.readErrno()
}

// Get sends a UAPI get command and returns the raw response
func (u *UAPI) Get() (string, error) {
	u.rw.WriteString("get=1\n\n")
	if err := u.rw.Flush(); err != nil {
		return "", fmt.Errorf("amnezia: flush get: %w", err)
	}

	var sb strings.Builder
	for {
		line, err := u.rw.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("amnezia: read get: %w", err)
		}
		if strings.HasPrefix(line, "errno=") {
			var code int
			fmt.Sscanf(strings.TrimSpace(line), "errno=%d", &code)
			u.rw.ReadString('\n') // trailing blank line
			if code != 0 {
				return "", fmt.Errorf("amnezia: get errno=%d", code)
			}
			return sb.String(), nil
		}
		sb.WriteString(line)
	}
}

// PeerStatus holds runtime info from a UAPI get response
type PeerStatus struct {
	LastHandshakeSec int64
	TxBytes          int64
	RxBytes          int64
}

// GetPeerStatus does a get and parses the first peer's status
func (u *UAPI) GetPeerStatus() (*PeerStatus, error) {
	raw, err := u.Get()
	if err != nil {
		return nil, err
	}

	ps := &PeerStatus{}
	for _, line := range strings.Split(raw, "\n") {
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "last_handshake_time_sec":
			fmt.Sscanf(val, "%d", &ps.LastHandshakeSec)
		case "tx_bytes":
			fmt.Sscanf(val, "%d", &ps.TxBytes)
		case "rx_bytes":
			fmt.Sscanf(val, "%d", &ps.RxBytes)
		}
	}
	return ps, nil
}

func (u *UAPI) readErrno() error {
	for {
		line, err := u.rw.ReadString('\n')
		if err != nil {
			return fmt.Errorf("amnezia: read errno: %w", err)
		}
		line = strings.TrimRight(line, "\n")
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "errno=") {
			var code int
			fmt.Sscanf(line, "errno=%d", &code)
			u.rw.ReadString('\n') // trailing blank line
			if code != 0 {
				return fmt.Errorf("amnezia: set errno=%d", code)
			}
			return nil
		}
	}
}
