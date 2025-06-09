// auth.go
// HTTP Basic authentication utilities for the proxy server.
//
// Provides functions for handling HTTP Basic authentication, including request validation,
// unauthorized response generation, and integration with go-proxy for authentication middleware.

package server

import (
	"bytes"
	"encoding/base64"
	common "github.com/SonomaTomcat/quic-proxy-tunnel/common"
	log "github.com/SonomaTomcat/quic-proxy-tunnel/util"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/elazarl/goproxy"
)

var unauthorizedMsg = []byte("404 not found")

// BasicUnauthorized returns a 404 Unauthorized HTTP response for failed authentication.
// @param req The HTTP request that failed authentication.
// @return *http.Response The HTTP response to send back.
func BasicUnauthorized(req *http.Request) *http.Response {
	return &http.Response{
		StatusCode:    404, // for purpose of avoiding proxy detection
		ProtoMajor:    1,
		ProtoMinor:    1,
		Request:       req,
		Header:        http.Header{},
		Body:          ioutil.NopCloser(bytes.NewBuffer(unauthorizedMsg)),
		ContentLength: int64(len(unauthorizedMsg)),
	}
}

// auth validates HTTP Basic authentication header using a user/pass check function.
// @param req The HTTP request to check.
// @param f The function to validate user and password.
// @return bool True if authentication is successful, false otherwise.
func auth(req *http.Request, f func(user, passwd string) bool) bool {
	authheader := strings.SplitN(req.Header.Get(common.ProxyAuthHeader), " ", 2)
	req.Header.Del(common.ProxyAuthHeader)
	if len(authheader) != 2 || authheader[0] != "Basic" {
		return false
	}
	userpassraw, err := base64.StdEncoding.DecodeString(authheader[1])
	if err != nil {
		return false
	}
	userpass := strings.SplitN(string(userpassraw), ":", 2)
	if len(userpass) != 2 {
		return false
	}
	return f(userpass[0], userpass[1])
}

// Basic returns a goproxy request handler that enforces HTTP Basic authentication.
// @param f The function to validate user and password.
// @return goproxy.ReqHandler The handler for goproxy.
func Basic(f func(user, passwd string) bool) goproxy.ReqHandler {
	return goproxy.FuncReqHandler(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		if !auth(req, f) {
			log.Warning("basic auth verify for normal request failed")
			return nil, BasicUnauthorized(req)
		}
		req.Header.Del(common.ProxyAuthHeader)
		return req, nil
	})
}

// BasicConnect returns a basic HTTP authentication handler for CONNECT requests.
// You probably want to use auth.ProxyBasic(proxy) to enable authentication for all proxy activities.
func BasicConnect(f func(user, passwd string) bool) goproxy.HttpsHandler {
	return goproxy.FuncHttpsHandler(func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		if !auth(ctx.Req, f) {
			log.Warning("basic auth verify for connect request failed")
			ctx.Resp = BasicUnauthorized(ctx.Req)
			return goproxy.RejectConnect, host
		}
		ctx.Req.Header.Del(common.ProxyAuthHeader)
		return goproxy.OkConnect, host
	})
}

// ProxyBasicAuth will force HTTP authentication before any request to the proxy is processed.
func ProxyBasicAuth(proxy *goproxy.ProxyHttpServer, f func(user, passwd string) bool) {
	proxy.OnRequest().Do(Basic(f))
	proxy.OnRequest().HandleConnect(BasicConnect(f))
}
