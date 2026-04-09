package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"path/filepath"

	"github.com/eduard256/jamperhub/internal/api"
	"github.com/eduard256/jamperhub/internal/balancer"
	"github.com/eduard256/jamperhub/internal/dhcp"
	"github.com/eduard256/jamperhub/internal/store"
	"github.com/eduard256/jamperhub/internal/tunnel/amnezia"
	"github.com/eduard256/jamperhub/internal/tunnel/xray"
	"github.com/eduard256/jamperhub/pkg/bindata"
	"github.com/eduard256/jamperhub/pkg/config"
)

const version = "0.0.1"

func main() {
	dataPath := flag.String("data", "/etc/jamperhub", "path to data directory")
	listenAddr := flag.String("listen", ":7891", "HTTP listen address")
	showVersion := flag.Bool("version", false, "show version")
	flag.Parse()

	if *showVersion {
		fmt.Println("JamperHUB v" + version)
		os.Exit(0)
	}

	log.Printf("[main] JamperHUB v%s starting, data=%s", version, *dataPath)

	// Step 1. Extract embedded binaries
	binDir := filepath.Join(*dataPath, "bin")
	if err := bindata.Extract(binDir); err != nil {
		log.Fatalf("[main] bindata: %v", err)
	}
	log.Printf("[main] binaries ready in %s", binDir)

	// Step 2. Load config
	if err := config.Init(*dataPath); err != nil {
		log.Fatalf("[main] config: %v", err)
	}
	log.Printf("[main] config loaded")

	// Step 3. Open database
	if err := store.Init(*dataPath); err != nil {
		log.Fatalf("[main] store: %v", err)
	}
	log.Printf("[main] database ready")

	// Step 4. Register tunnel types
	amnezia.Init()
	xray.Init()

	// Step 5. Init modules
	dhcp.Init()
	balancer.Init()

	// Step 6. Start API server (serves UI + API)
	api.Init(*listenAddr)

	log.Printf("[main] ready, listening on %s", *listenAddr)

	// Block until signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Printf("[main] shutting down")
}
