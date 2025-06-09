package client

import (
	"github.com/SonomaTomcat/quic-proxy-tunnel/common"
	log "github.com/SonomaTomcat/quic-proxy-tunnel/util"
	"github.com/elazarl/goproxy"
	"net"
	"net/http"
	"strconv"
	"strings"
)

func RunHttpProxy(localPort int, dialer *common.QuicDialer, remoteAddr string, logLevel string) {
	proxy := goproxy.NewProxyHttpServer()
	if lvl := strings.ToUpper(logLevel); lvl == "DEBUG" || lvl == "INFO" {
		proxy.Verbose = true
	} else {
		proxy.Verbose = false
	}
	proxy.Tr = &http.Transport{
		Dial: func(network, addr string) (net.Conn, error) {
			return dialer.Dial(remoteAddr, addr)
		},
	}
	proxy.ConnectDial = func(network, addr string) (net.Conn, error) {
		return dialer.Dial(remoteAddr, addr)
	}
	listenAddr := ":" + strconv.Itoa(localPort)
	log.Info("start http proxy on %s", listenAddr)
	if err := http.ListenAndServe(listenAddr, proxy); err != nil {
		log.Error("http proxy error: %v", err)
	}
}

