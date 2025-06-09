// server.go
// QUIC proxy server that accepts connections from clients and forwards traffic to target addresses.
//
// Loads configuration, listens for QUIC connections, and relays data between client and target.
package server

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/SonomaTomcat/quic-proxy-tunnel/common"
	log "github.com/SonomaTomcat/quic-proxy-tunnel/util"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config represents the server configuration loaded from a JSON file.
type Config struct {
	ListenPort int    `json:"listen"`
	Cert       string `json:"cert"`
	Key        string `json:"key"`
	Auth       string `json:"auth"`
	LogLevel   string `json:"logLevel"`
	Profile    bool   `json:"profile"`
}

// Run starts the QUIC proxy server using the provided configuration file path.
//
// It loads the configuration, sets up the server, and begins accepting and handling client connections.
func Run(configPath string) {
	fmt.Println("[DEBUG] server.Run called, configPath:", configPath)
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

	fmt.Println("[DEBUG] config loaded:", cfg)
	log.Info("[DEBUG] log.Info test output")

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

	if cfg.Cert == "" || cfg.Key == "" {
		log.Error("cert and key can't be empty")
		return
	}

	parts := strings.Split(cfg.Auth, ":")
	if len(parts) != 2 {
		log.Error("auth param invalid")
		return
	}

	listenAddr := ":" + strconv.Itoa(cfg.ListenPort)

	if cfg.Profile {
		pprofAddr := "localhost:6060"
		log.Notice("listen pprof:%s", pprofAddr)
		go func() {
			err := http.ListenAndServe(pprofAddr, nil)
			if err != nil {
				log.Error("listen pprof failed:%v", err)
				return
			}
		}()
	}

	listener, err := common.ListenAddr(listenAddr, generateTLSConfig(cfg.Cert, cfg.Key))
	if err != nil {
		log.Error("listen failed:%v", err)
		return
	}
	ql := common.NewQuicListener(listener)

	for {
		conn, err := ql.Accept()
		if err != nil {
			log.Error("quic accept error: %v", err)
			continue
		}

		go func(conn net.Conn) {
			log.Debug("server accepted new connection from %s", conn.RemoteAddr())
			defer conn.Close()
			reader := bufio.NewReader(conn)
			targetAddr, err := reader.ReadString('\n')
			if err != nil {
				log.Error("failed to read target address: %v", err)
				return
			}
			targetAddr = strings.TrimSpace(targetAddr)
			log.Info("server got new stream, target: %s", targetAddr)

			if targetAddr == "__UDP__" {
				// Handle UDP over QUIC
				for {
					// Read UDP packet: [dest addr len 1 byte][dest addr string][data len 2 bytes][data]
					head := make([]byte, 1)
					_, err := io.ReadFull(reader, head)
					if err != nil {
						log.Error("failed to read UDP dest addr len: %v", err)
						return
					}
					destLen := int(head[0])
					destBuf := make([]byte, destLen)
					_, err = io.ReadFull(reader, destBuf)
					if err != nil {
						log.Error("failed to read UDP dest addr: %v", err)
						return
					}
					lenBuf := make([]byte, 2)
					_, err = io.ReadFull(reader, lenBuf)
					if err != nil {
						log.Error("failed to read UDP data len: %v", err)
						return
					}
					dataLen := int(lenBuf[0])<<8 | int(lenBuf[1])
					data := make([]byte, dataLen)
					_, err = io.ReadFull(reader, data)
					if err != nil {
						log.Error("failed to read UDP data: %v", err)
						return
					}
					destAddr := string(destBuf)
					udpAddr, err := net.ResolveUDPAddr("udp", destAddr)
					if err != nil {
						log.Error("failed to resolve UDP dest addr %s: %v", destAddr, err)
						return
					}
					udpConn, err := net.DialUDP("udp", nil, udpAddr)
					if err != nil {
						log.Error("failed to dial UDP %s: %v", destAddr, err)
						return
					}
					_, err = udpConn.Write(data)
					if err != nil {
						log.Error("failed to send UDP data to %s: %v", destAddr, err)
						udpConn.Close()
						return
					}

					// Read UDP response from the target and send it back to the client over QUIC
					respBuf := make([]byte, 65535)
					udpConn.SetReadDeadline(time.Now().Add(2 * time.Second)) // avoid blocking forever
					n, _, err := udpConn.ReadFrom(respBuf)
					if err == nil && n > 0 {
						// Send response back to client using the same format
						// [data len 2 bytes][data]
						respLen := n
						lenBytes := []byte{byte(respLen >> 8), byte(respLen & 0xff)}
						conn.Write(lenBytes)
						conn.Write(respBuf[:respLen])
					}

					udpConn.Close()
				}
				return
			}

			remote, err := net.Dial("tcp", targetAddr)
			if err != nil {
				log.Error("failed to dial target %s: %v", targetAddr, err)
				return
			}
			log.Debug("server connected to target %s", targetAddr)
			defer remote.Close()

			// Bidirectional forwarding
			go func() {
				written, _ := io.Copy(remote, reader)
				log.Debug("copied %d bytes from client to target %s", written, targetAddr)
			}()
			read, _ := io.Copy(conn, remote)
			log.Debug("copied %d bytes from target %s to client ", read, targetAddr)
		}(conn)
	}
}

// generateTLSConfig loads the TLS certificate and key for the server.
func generateTLSConfig(certFile, keyFile string) *tls.Config {
	tlsCert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		panic(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{common.KQuicProxy},
	}
}
