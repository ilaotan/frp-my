// Copyright 2017 fatedier, fatedier@gmail.com
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package vhost

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	stdlog "log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	libio "github.com/fatedier/golib/io"
	"github.com/fatedier/golib/pool"

	httppkg "github.com/fatedier/frp/pkg/util/http"
	"github.com/fatedier/frp/pkg/util/log"
	"github.com/fatedier/frp/pkg/util/util"
)

var ErrNoRouteFound = errors.New("no route found")

const (
	defaultAuthHeader      = "Authorization"
	defaultProxyAuthHeader = "Proxy-Authorization"
)

type HTTPReverseProxyOptions struct {
	ResponseHeaderTimeoutS int64
}

type HTTPReverseProxy struct {
	proxy       http.Handler
	vhostRouter *Routers

	responseHeaderTimeout time.Duration
}

func NewHTTPReverseProxy(option HTTPReverseProxyOptions, vhostRouter *Routers) *HTTPReverseProxy {
	if option.ResponseHeaderTimeoutS <= 0 {
		option.ResponseHeaderTimeoutS = 60
	}
	rp := &HTTPReverseProxy{
		responseHeaderTimeout: time.Duration(option.ResponseHeaderTimeoutS) * time.Second,
		vhostRouter:           vhostRouter,
	}
	proxy := &httputil.ReverseProxy{
		// Modify incoming requests by route policies.
		Rewrite: func(r *httputil.ProxyRequest) {
			r.Out.Header["X-Forwarded-For"] = r.In.Header["X-Forwarded-For"]
			r.SetXForwarded()
			req := r.Out
			req.URL.Scheme = "http"
			reqRouteInfo := req.Context().Value(RouteInfoKey).(*RequestRouteInfo)
			originalHost, _ := httppkg.CanonicalHost(reqRouteInfo.Host)

			rc := req.Context().Value(RouteConfigKey).(*RouteConfig)
			if rc != nil {
				// Don't forward the credentials used to authenticate against frp to the
				// local service, which may rely on the same header for its own auth.
				if rc.authEnabled() {
					req.Header.Del(rc.authHeaderName(reqRouteInfo.URLHost != ""))
				}
				if rc.authPageEnabled() {
					stripAuthCookie(req.Header, rc.authCookieName())
				}

				if rc.RewriteHost != "" {
					req.Host = rc.RewriteHost
				}

				var endpoint string
				if rc.ChooseEndpointFn != nil {
					// ignore error here, it will use CreateConnFn instead later
					endpoint, _ = rc.ChooseEndpointFn()
					reqRouteInfo.Endpoint = endpoint
					log.Tracef("choose endpoint name [%s] for http request host [%s] path [%s] httpuser [%s]",
						endpoint, originalHost, reqRouteInfo.URL, reqRouteInfo.HTTPUser)
				}
				// Set {domain}.{location}.{routeByHTTPUser}.{endpoint} as URL host here to let http transport reuse connections.
				req.URL.Host = rc.Domain + "." +
					base64.StdEncoding.EncodeToString([]byte(rc.Location)) + "." +
					base64.StdEncoding.EncodeToString([]byte(rc.RouteByHTTPUser)) + "." +
					base64.StdEncoding.EncodeToString([]byte(endpoint))

				for k, v := range rc.Headers {
					req.Header.Set(k, v)
				}
			} else {
				req.URL.Host = req.Host
			}
		},
		ModifyResponse: func(r *http.Response) error {
			rc := r.Request.Context().Value(RouteConfigKey).(*RouteConfig)
			if rc != nil {
				for k, v := range rc.ResponseHeaders {
					r.Header.Set(k, v)
				}
			}
			return nil
		},
		// Create a connection to one proxy routed by route policy.
		Transport: &http.Transport{
			ResponseHeaderTimeout: rp.responseHeaderTimeout,
			IdleConnTimeout:       60 * time.Second,
			MaxIdleConnsPerHost:   5,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return rp.CreateConnection(ctx.Value(RouteInfoKey).(*RequestRouteInfo), true)
			},
			Proxy: func(req *http.Request) (*url.URL, error) {
				// Use proxy mode if there is host in HTTP first request line.
				// GET http://example.com/ HTTP/1.1
				// Host: example.com
				//
				// Normal:
				// GET / HTTP/1.1
				// Host: example.com
				urlHost := req.Context().Value(RouteInfoKey).(*RequestRouteInfo).URLHost
				if urlHost != "" {
					return req.URL, nil
				}
				return nil, nil
			},
		},
		BufferPool: pool.NewBuffer(32 * 1024),
		ErrorLog:   stdlog.New(log.NewWriteLogger(log.WarnLevel, 2), "", 0),
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, err error) {
			log.Logf(log.WarnLevel, 1, "do http proxy request [host: %s] error: %v", req.Host, err)
			if err != nil {
				if e, ok := err.(net.Error); ok && e.Timeout() {
					rw.WriteHeader(http.StatusGatewayTimeout)
					return
				}
			}
			rw.WriteHeader(http.StatusNotFound)
			_, _ = rw.Write(getNotFoundPageContent())
		},
	}
	rp.proxy = proxy
	return rp
}

// Register register the route config to reverse proxy
// reverse proxy will use CreateConnFn from routeCfg to create a connection to the remote service
func (rp *HTTPReverseProxy) Register(routeCfg RouteConfig) error {
	err := rp.vhostRouter.Add(routeCfg.Domain, routeCfg.Location, routeCfg.RouteByHTTPUser, &routeCfg)
	if err != nil {
		return err
	}
	return nil
}

// UnRegister unregister route config by domain and location
func (rp *HTTPReverseProxy) UnRegister(routeCfg RouteConfig) {
	rp.vhostRouter.Del(routeCfg.Domain, routeCfg.Location, routeCfg.RouteByHTTPUser)
}

func (rp *HTTPReverseProxy) GetRouteConfig(domain, location, routeByHTTPUser string) *RouteConfig {
	vr, ok := rp.vhostRouter.getByRoute(domain, location, routeByHTTPUser)
	if ok {
		log.Debugf("get new http request host [%s] path [%s] httpuser [%s]", domain, location, routeByHTTPUser)
		return vr.payload.(*RouteConfig)
	}
	return nil
}

// CreateConnection create a new connection by route config
func (rp *HTTPReverseProxy) CreateConnection(reqRouteInfo *RequestRouteInfo, byEndpoint bool) (net.Conn, error) {
	host, _ := httppkg.CanonicalHost(reqRouteInfo.Host)
	vr, ok := rp.vhostRouter.getByRoute(host, reqRouteInfo.URL, reqRouteInfo.HTTPUser)
	if ok {
		if byEndpoint {
			fn := vr.payload.(*RouteConfig).CreateConnByEndpointFn
			if fn != nil {
				return fn(reqRouteInfo.Endpoint, reqRouteInfo.RemoteAddr)
			}
		}
		fn := vr.payload.(*RouteConfig).CreateConnFn
		if fn != nil {
			return fn(reqRouteInfo.RemoteAddr)
		}
	}
	return nil, fmt.Errorf("%v: %s %s %s", ErrNoRouteFound, host, reqRouteInfo.URL, reqRouteInfo.HTTPUser)
}

func checkRouteAllowIP(req *http.Request, rc *RouteConfig) bool {
	if rc == nil || rc.AllowList.Empty() {
		return true
	}
	return rc.AllowList.ContainsRemoteAddr(req.RemoteAddr)
}

// authEnabled reports whether this route requires client authentication.
func (rc *RouteConfig) authEnabled() bool {
	return rc != nil && (rc.Username != "" || rc.Password != "")
}

// authHeaderName returns the header carrying the client credentials.
//
// A custom header can be configured so that frp authentication doesn't occupy
// the standard Authorization header, which many local services use for their
// own authentication.
func (rc *RouteConfig) authHeaderName(proxyMode bool) string {
	if rc != nil && rc.AuthHeader != "" {
		return rc.AuthHeader
	}
	if proxyMode {
		return defaultProxyAuthHeader
	}
	return defaultAuthHeader
}

// parseAuthCredentials parses `Basic base64(user:pass)`.
// A bare base64 value is also accepted for convenience with custom headers.
func parseAuthCredentials(value string) (username, password string, ok bool) {
	if username, password, ok = httppkg.ParseBasicAuth(value); ok {
		return username, password, true
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", "", false
	}
	before, after, found := strings.Cut(string(decoded), ":")
	if !found {
		return "", "", false
	}
	return before, after, true
}

func checkRouteAuthByRequest(req *http.Request, rc *RouteConfig) bool {
	if !rc.authEnabled() {
		return true
	}

	// A session created on the frps login page is as good as sending the header.
	if rc.hasValidAuthCookie(req) {
		return true
	}

	value := req.Header.Get(rc.authHeaderName(req.URL.Host != ""))
	if value == "" {
		return false
	}
	user, passwd, ok := parseAuthCredentials(value)
	if !ok {
		return false
	}
	return util.ConstantTimeEqString(user, rc.Username) &&
		util.ConstantTimeEqString(passwd, rc.Password)
}

func (rp *HTTPReverseProxy) connectHandler(rw http.ResponseWriter, req *http.Request) {
	if req.ContentLength > 0 || len(req.TransferEncoding) > 0 {
		http.Error(rw, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	hj, ok := rw.(http.Hijacker)
	if !ok {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Forward only CONNECT headers; the original Body must not be used after Hijack.
	outreq := req.Clone(req.Context())
	outreq.Body = nil
	outreq.GetBody = nil
	outreq.ContentLength = 0
	outreq.TransferEncoding = nil
	outreq.Trailer = nil
	outreq.Header.Del("Content-Length")
	outreq.Header.Del("Transfer-Encoding")
	outreq.Header.Del("Trailer")

	client, buffered, err := hj.Hijack()
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	remote, err := rp.CreateConnection(req.Context().Value(RouteInfoKey).(*RequestRouteInfo), false)
	if err != nil {
		_ = NotFoundResponse().Write(client)
		client.Close()
		return
	}
	if err := outreq.Write(remote); err != nil {
		_ = remote.Close()
		_ = client.Close()
		return
	}
	go libio.Join(remote, libio.WrapReadWriteCloser(buffered.Reader, client, client.Close))
}

func getRequestRouteUser(req *http.Request) string {
	if req.URL.Host != "" {
		proxyAuth := req.Header.Get("Proxy-Authorization")
		if proxyAuth == "" {
			// Preserve legacy proxy-mode routing when clients send only Authorization,
			// so requests still hit the matched route and return 407 instead of 404.
			// Auth validation intentionally does not share this fallback.
			user, _, _ := req.BasicAuth()
			return user
		}
		user, _, _ := httppkg.ParseBasicAuth(proxyAuth)
		return user
	}
	user, _, _ := req.BasicAuth()
	return user
}

func (rp *HTTPReverseProxy) injectRequestInfoToCtx(req *http.Request) *http.Request {
	user := getRequestRouteUser(req)

	reqRouteInfo := &RequestRouteInfo{
		URL:        req.URL.Path,
		Host:       req.Host,
		HTTPUser:   user,
		RemoteAddr: req.RemoteAddr,
		URLHost:    req.URL.Host,
	}

	originalHost, _ := httppkg.CanonicalHost(reqRouteInfo.Host)
	rc := rp.GetRouteConfig(originalHost, reqRouteInfo.URL, reqRouteInfo.HTTPUser)

	newctx := req.Context()
	newctx = context.WithValue(newctx, RouteInfoKey, reqRouteInfo)
	newctx = context.WithValue(newctx, RouteConfigKey, rc)
	return req.Clone(newctx)
}

func (rp *HTTPReverseProxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	newreq := rp.injectRequestInfoToCtx(req)
	rc := newreq.Context().Value(RouteConfigKey).(*RouteConfig)

	// Check the client IP before auth so that rejected clients can't probe
	// credentials or tell an existing route from a missing one.
	if !checkRouteAllowIP(req, rc) {
		log.Warnf("http request from [%s] for host [%s] path [%s] is not in allowIPs, rejected",
			req.RemoteAddr, req.Host, req.URL.Path)
		http.Error(rw, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}

	// Serve the frps login page on the reserved paths.
	if rc.authPageEnabled() && rc.isAuthPagePath(req.URL.Path) {
		rp.serveAuthPage(rw, req, rc)
		return
	}

	if !checkRouteAuthByRequest(req, rc) {
		switch {
		case req.URL.Host != "":
			rw.Header().Set("Proxy-Authenticate", `Basic realm="Restricted"`)
			http.Error(rw, http.StatusText(http.StatusProxyAuthRequired), http.StatusProxyAuthRequired)
		case rc.authPageEnabled() && wantsHTML(req) &&
			(req.Method == http.MethodGet || req.Method == http.MethodHead):
			// A browser can't send a custom header, so send it to the login form.
			http.Redirect(rw, req, rc.authLoginPath()+"?redirect="+url.QueryEscape(req.URL.RequestURI()), http.StatusFound)
		case rc.AuthHeader != "":
			// API clients get a plain 401 without a challenge, so no browser
			// dialog is triggered.
			log.Debugf("http request for host [%s] path [%s] is missing credentials in header [%s]",
				req.Host, req.URL.Path, rc.AuthHeader)
			http.Error(rw, "401 Unauthorized: credentials are required in the "+rc.AuthHeader+" header", http.StatusUnauthorized)
		default:
			rw.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			http.Error(rw, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		}
		return
	}

	if req.Method == http.MethodConnect {
		rp.connectHandler(rw, newreq)
	} else {
		rp.proxy.ServeHTTP(rw, newreq)
	}
}
