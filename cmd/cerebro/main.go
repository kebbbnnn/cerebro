package main

import (
	"log"
	"os"

	"github.com/kebbbnnn/cerebro/internal/server"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmsgprefix)
	log.SetPrefix("[cerebro] ")

	// Determine config file path.
	configPath := os.Getenv("CEREBRO_CONFIG")
	if configPath == "" {
		configPath = "config.yaml"
	}

	// Load and validate configuration.
	cfg, err := server.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Start the server using the configuration.
	server.Run(cfg)
}
