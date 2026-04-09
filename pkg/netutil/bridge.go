package netutil

import "fmt"

// CreateBridge creates a Linux bridge interface
func CreateBridge(name string) error {
	if err := run("ip", "link", "add", name, "type", "bridge"); err != nil {
		return fmt.Errorf("create bridge %s: %w", name, err)
	}
	if err := run("ip", "link", "set", name, "up"); err != nil {
		return fmt.Errorf("bring up bridge %s: %w", name, err)
	}
	return nil
}

// DeleteBridge removes a Linux bridge interface
func DeleteBridge(name string) error {
	if err := run("ip", "link", "set", name, "down"); err != nil {
		return fmt.Errorf("bring down bridge %s: %w", name, err)
	}
	if err := run("ip", "link", "del", name); err != nil {
		return fmt.Errorf("delete bridge %s: %w", name, err)
	}
	return nil
}
