package healthcheck

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

// Ping sends a quick HTTP request through a tunnel interface or SOCKS5 proxy.
// Returns latency in ms, or error if unreachable.
func Ping(testURL, tunIface string, isTUN bool, socksPort int, timeout time.Duration) (int, error) {
	start := time.Now()

	var client *http.Client
	if isTUN {
		// route through TUN interface using --interface
		client = &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   timeout,
					LocalAddr: nil,
					Control:   bindToDevice(tunIface),
				}).DialContext,
			},
		}
	} else {
		// route through SOCKS5 proxy
		client = &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				Proxy: http.ProxyURL(mustParseURL(fmt.Sprintf("socks5://127.0.0.1:%d", socksPort))),
			},
		}
	}

	resp, err := client.Get(testURL)
	if err != nil {
		return 0, err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	latency := int(time.Since(start).Milliseconds())
	return latency, nil
}

// PingViaCurl uses curl as a fallback for healthcheck.
// More reliable for TUN interfaces where Go's SO_BINDTODEVICE may not work without CAP_NET_RAW.
func PingViaCurl(testURL, tunIface string, isTUN bool, socksPort int, timeout time.Duration) (int, error) {
	var args []string
	if isTUN {
		args = []string{"--interface", tunIface}
	} else {
		args = []string{"-x", fmt.Sprintf("socks5h://127.0.0.1:%d", socksPort)}
	}
	args = append(args, "-s", "-o", "/dev/null", "-w", "%{time_total}",
		"--max-time", fmt.Sprintf("%d", int(timeout.Seconds())),
		testURL)

	start := time.Now()
	out, err := exec.Command("curl", args...).CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("curl: %s: %w", strings.TrimSpace(string(out)), err)
	}

	latency := int(time.Since(start).Milliseconds())
	return latency, nil
}

// SpeedTest downloads a file through the tunnel and returns speed in Mbps.
func SpeedTest(url, tunIface string, isTUN bool, socksPort int, timeout time.Duration) (float64, error) {
	var args []string
	if isTUN {
		args = []string{"--interface", tunIface}
	} else {
		args = []string{"-x", fmt.Sprintf("socks5h://127.0.0.1:%d", socksPort)}
	}
	args = append(args, "-s", "-o", "/dev/null", "-w", "%{size_download} %{time_total}",
		"--max-time", fmt.Sprintf("%d", int(timeout.Seconds())),
		url)

	out, err := exec.Command("curl", args...).CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("speed test: %s: %w", strings.TrimSpace(string(out)), err)
	}

	var bytes float64
	var seconds float64
	fmt.Sscanf(strings.TrimSpace(string(out)), "%f %f", &bytes, &seconds)
	if seconds <= 0 {
		return 0, fmt.Errorf("speed test: invalid time")
	}

	mbps := (bytes * 8) / (seconds * 1_000_000)
	return mbps, nil
}

// GetTrafficRate reads current rx+tx bytes from /sys/class/net/<iface>/statistics
func GetTrafficRate(iface string, prevBytes int64, interval time.Duration) (currentBytes int64, kbps float64) {
	rx := readSysInt("/sys/class/net/" + iface + "/statistics/rx_bytes")
	tx := readSysInt("/sys/class/net/" + iface + "/statistics/tx_bytes")
	currentBytes = rx + tx

	if prevBytes > 0 && interval > 0 {
		diff := float64(currentBytes - prevBytes)
		kbps = diff / interval.Seconds() / 1024
	}
	return
}

// internals

func readSysInt(path string) int64 {
	out, err := exec.Command("cat", path).Output()
	if err != nil {
		return 0
	}
	var val int64
	fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &val)
	return val
}

func mustParseURL(raw string) *url.URL {
	u, _ := url.Parse(raw)
	return u
}
