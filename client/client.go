// client.go
// HTTP proxy client that tunnels traffic over a QUIC connection to a remote server.
//
// Loads configuration, sets up a local HTTP proxy, and forwards requests via QUIC.
package client

import (
	"encoding/json"
	"fmt"
	"github.com/SonomaTomcat/quic-proxy-tunnel/common"
	log "github.com/SonomaTomcat/quic-proxy-tunnel/util"
	"os"
	"strings"
)

type Config struct {
	LocalPort      int    `json:"listen"`
	RemoteAddr     string `json:"remoteAddr"`
	SkipCertVerify bool   `json:"skipCertVerify"`
	Auth           string `json:"auth"`
	LogLevel       string `json:"logLevel"`
	Profile        bool   `json:"profile"`
}

// Run initializes the HTTP proxy client using the provided configuration file.
//
// It reads the configuration, validates it, and starts an HTTP proxy server
// that forwards traffic to a remote QUIC server.
func Run(configPath string) {
	file, err := os.Open(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open config file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	var cfg Config
	if err := json.NewDecoder(file).Decode(&cfg); err != nil {
		fmt.Fprintf(os.Stderr, "failed to decode config: %v\n", err)
		os.Exit(1)
	}

	// Set log level based on configuration
	switch strings.ToUpper(cfg.LogLevel) {
	case "DEBUG":
		log.SetLevel(log.DEBUG)
	case "INFO":
		log.SetLevel(log.INFO)
	case "NOTICE":
		log.SetLevel(log.NOTICE)
	case "WARNING":
		log.SetLevel(log.WARNING)
	case "ERROR":
		log.SetLevel(log.ERROR)
	case "CRITICAL":
		log.SetLevel(log.CRITICAL)
	default:
		log.SetLevel(log.INFO)
	}

	if cfg.RemoteAddr == "" {
		log.Error("remote quic server address required")
		return
	}

	parts := strings.Split(cfg.Auth, ":")
	if len(parts) != 2 {
		log.Error("auth param invalid")
		return
	}

	dialer := common.NewQuicDialer(cfg.SkipCertVerify)

	RunHttpProxy(cfg.LocalPort, dialer, cfg.RemoteAddr, cfg.LogLevel)
}

