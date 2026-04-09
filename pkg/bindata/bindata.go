package bindata

import (
	"compress/gzip"
	"embed"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

//go:embed amd64/*.gz
var embedded embed.FS

// binary name -> embedded path
var bins = map[string]string{
	"amneziawg-go": "amd64/amneziawg-go.gz",
	"xray":         "amd64/xray.gz",
	"tun2socks":    "amd64/tun2socks.gz",
}

// Extract unpacks all embedded binaries into binDir.
// Skips files that already exist.
func Extract(binDir string) error {
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("bindata: mkdir: %w", err)
	}

	for name, src := range bins {
		dst := filepath.Join(binDir, name)

		// skip if already extracted
		if _, err := os.Stat(dst); err == nil {
			continue
		}

		if err := extractOne(src, dst); err != nil {
			return fmt.Errorf("bindata: extract %s: %w", name, err)
		}
		log.Printf("[bindata] extracted %s", name)
	}
	return nil
}

func extractOne(src, dst string) error {
	f, err := embedded.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, gz)
	return err
}

// BinPath returns the full path to an embedded binary
func BinPath(binDir, name string) string {
	return filepath.Join(binDir, name)
}
