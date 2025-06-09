// main.go
// Entry point for the QUIC proxy tunnel application.
//
// Parses command-line arguments and starts the client or server mode based on user input.
package main

import (
	"fmt"
	"github.com/SonomaTomcat/quic-proxy-tunnel/client"
	"github.com/SonomaTomcat/quic-proxy-tunnel/server"
	"os"
	"path/filepath"
)

func printHelp() {
	fmt.Printf(`Usage: %s <command> [--config <config.json>]

Commands:
  client     Run in client mode
  server     Run in server mode

Options:
  -c, --config   Specify config file (default: test/client.json or test/server.json)
  -h, --help Show this help message
`, filepath.Base(os.Args[0]))
}

func main() {
	if len(os.Args) < 2 || os.Args[1] == "-h" || os.Args[1] == "--help" || os.Args[1] == "" || os.Args[1] == "help" {
		printHelp()
		os.Exit(0)
	}
	mode := os.Args[1]
	var configPath string
	for i, arg := range os.Args {
		if (arg == "--config" || arg == "-c") && i+1 < len(os.Args) {
			configPath = os.Args[i+1]
		}
	}
	if configPath == "" {
		if mode == "client" {
			configPath = "test/client.json"
		} else if mode == "server" {
			configPath = "test/server.json"
		}
	}

	switch mode {
	case "client":
		client.Run(configPath)
	case "server":
		server.Run(configPath)
	default:
		fmt.Fprintf(os.Stderr, "Unknown mode: %s\n", mode)
		printHelp()
		os.Exit(1)
	}
}
