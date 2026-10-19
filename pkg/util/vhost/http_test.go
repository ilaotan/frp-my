package vhost

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	httppkg "github.com/fatedier/frp/pkg/util/http"
	netpkg "github.com/fatedier/frp/pkg/util/net"
)

func TestHTTPConnectTunnel(t *testing.T) {
	for _, tc := range []struct {
		name      string
		framing   string
		pipelined bool
	}{
		{name: "plain"},
		{name: "plain-early", pipelined: true},
		{name: "cl0", framing: "Content-Length: 0\r\n"},
		{name: "cl0-early", framing: "Content-Length: 0\r\n", pipelined: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			remote, backend := net.Pipe()
			defer remote.Close()
			defer backend.Close()
			require.NoError(t, backend.SetDeadline(time.Now().Add(5*time.Second)))

			rp := NewHTTPReverseProxy(HTTPReverseProxyOptions{}, NewRouters())
			require.NoError(t, rp.Register(RouteConfig{
				Domain:   "target.example.com",
				Username: "alice",
				Password: "secret",
				CreateConnFn: func(string) (net.Conn, error) {
					return remote, nil
				},
			}))
			listener := netpkg.NewInternalListener()
			server := &http.Server{Handler: rp, ReadHeaderTimeout: time.Second}
			serveErr := make(chan error, 1)
			go func() {
				serveErr <- server.Serve(listener)
			}()
			defer func() {
				require.NoError(t, server.Close())
				require.ErrorIs(t, <-serveErr, http.ErrServerClosed)
			}()

			client, incoming := net.Pipe()
			defer client.Close()
			defer incoming.Close()
			require.NoError(t, client.SetDeadline(time.Now().Add(5*time.Second)))
			require.NoError(t, listener.PutConn(incoming))

			proxyAuth := httppkg.BasicAuth("alice", "secret")
			request := "CONNECT target.example.com:443 HTTP/1.1\r\nHost: target.example.com:443\r\n" +
				"X-Connect-Test: tunnel\r\nProxy-Authorization: " + proxyAuth + "\r\n" + tc.framing + "\r\n"
			if tc.pipelined {
				// One pipe write puts the tunnel data in net/http's request buffer.
				request += "early data"
			}
			_, err := io.WriteString(client, request)
			require.NoError(t, err)
			backendReader := bufio.NewReader(backend)
			req, err := http.ReadRequest(backendReader)
			require.NoError(t, err)
			defer req.Body.Close()
			require.Equal(t, http.MethodConnect, req.Method)
			require.Equal(t, "target.example.com:443", req.Host)
			require.Equal(t, "target.example.com:443", req.URL.Host)
			require.Equal(t, "tunnel", req.Header.Get("X-Connect-Test"))
			require.Equal(t, proxyAuth, req.Header.Get("Proxy-Authorization"))
			require.Zero(t, req.ContentLength)
			require.Empty(t, req.TransferEncoding)
			if tc.pipelined {
				data := make([]byte, len("early data"))
				_, err = io.ReadFull(backendReader, data)
				require.NoError(t, err)
				require.Equal(t, "early data", string(data))
			}

			writeErr := make(chan error, 1)
			go func() {
				_, err := io.WriteString(backend, "HTTP/1.1 200 Connection Established\r\n\r\nreply")
				writeErr <- err
			}()
			clientReader := bufio.NewReader(client)
			response, err := http.ReadResponse(clientReader, req)
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, http.StatusOK, response.StatusCode)
			reply := make([]byte, len("reply"))
			_, err = io.ReadFull(clientReader, reply)
			require.NoError(t, err)
			require.Equal(t, "reply", string(reply))
			require.NoError(t, <-writeErr)

			_, err = io.WriteString(client, "later data")
			require.NoError(t, err)
			data := make([]byte, len("later data"))
			_, err = io.ReadFull(backendReader, data)
			require.NoError(t, err)
			require.Equal(t, "later data", string(data))
		})
	}
}

func TestHTTPConnectRejectsContent(t *testing.T) {
	for _, tc := range []struct {
		name    string
		framing string
		body    string
	}{
		{name: "content-length", framing: "Content-Length: 1\r\n", body: "X"},
		{name: "chunked", framing: "Transfer-Encoding: chunked\r\n", body: "1\r\nX\r\n0\r\n\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backendCalls := 0
			rp := NewHTTPReverseProxy(HTTPReverseProxyOptions{}, NewRouters())
			require.NoError(t, rp.Register(RouteConfig{
				Domain: "target.example.com",
				CreateConnFn: func(string) (net.Conn, error) {
					backendCalls++
					return nil, io.ErrClosedPipe
				},
			}))
			req, err := http.ReadRequest(bufio.NewReader(strings.NewReader(
				"CONNECT target.example.com:443 HTTP/1.1\r\nHost: target.example.com:443\r\n" + tc.framing + "\r\n" + tc.body)))
			require.NoError(t, err)
			defer req.Body.Close()
			rw := &connectTestResponseWriter{ResponseRecorder: httptest.NewRecorder()}

			rp.ServeHTTP(rw, req)

			require.Equal(t, http.StatusBadRequest, rw.Code)
			require.Zero(t, rw.hijackCalls, "reject content before Hijack")
			require.Zero(t, backendCalls, "reject content before creating a backend")
		})
	}
}

func TestHTTPConnectHeaderWriteFailure(t *testing.T) {
	for _, originalBody := range []bool{false, true} {
		t.Run(fmt.Sprintf("originalBody=%v", originalBody), func(t *testing.T) {
			client, incoming := net.Pipe()
			defer client.Close()
			defer incoming.Close()
			remote, backend := net.Pipe()
			defer remote.Close()
			defer backend.Close()
			clientConn := &connectTestConn{Conn: incoming}
			remoteConn := &connectTestConn{Conn: remote, failWrites: true}
			rp := NewHTTPReverseProxy(HTTPReverseProxyOptions{}, NewRouters())
			require.NoError(t, rp.Register(RouteConfig{
				Domain: "target.example.com",
				CreateConnFn: func(string) (net.Conn, error) {
					return remoteConn, nil
				},
			}))
			req := rp.injectRequestInfoToCtx(httptest.NewRequest(http.MethodConnect, "target.example.com:443", nil))
			require.Equal(t, "target.example.com:443", req.Host)
			body := &connectTestBody{}
			if originalBody {
				req.Body = body
			}
			requestBody := req.Body
			getBodyCalls := 0
			req.GetBody = func() (io.ReadCloser, error) {
				getBodyCalls++
				return body, nil
			}
			req.Header.Set("Content-Length", "0")
			req.Header.Set("Trailer", "X-Connect-Trailer")
			req.Trailer = http.Header{"X-Connect-Trailer": {"value"}}
			originalHeader, originalTrailer := req.Header.Clone(), req.Trailer.Clone()
			rw := &connectTestResponseWriter{
				ResponseRecorder: httptest.NewRecorder(),
				conn:             clientConn,
				buffered:         bufio.NewReadWriter(bufio.NewReader(clientConn), bufio.NewWriter(clientConn)),
			}

			rp.connectHandler(rw, req)

			require.Equal(t, 1, rw.hijackCalls)
			require.Zero(t, body.reads, "do not read the original Body after Hijack")
			require.Zero(t, body.closes, "do not close the original Body after Hijack")
			require.Positive(t, remoteConn.writes.Load(), "exercise CONNECT header write failure")
			require.True(t, remoteConn.closed.Load(), "close backend on CONNECT header write failure")
			require.True(t, clientConn.closed.Load(), "close client on CONNECT header write failure")
			require.Zero(t, remoteConn.reads.Load(), "do not start Join after header write failure")
			require.Zero(t, clientConn.reads.Load(), "do not start Join after header write failure")
			require.Zero(t, getBodyCalls)
			require.Equal(t, requestBody, req.Body)
			require.NotNil(t, req.GetBody)
			require.Equal(t, originalHeader, req.Header)
			require.Equal(t, originalTrailer, req.Trailer)
		})
	}
}

type connectTestResponseWriter struct {
	*httptest.ResponseRecorder
	conn        net.Conn
	buffered    *bufio.ReadWriter
	hijackCalls int
}

func (rw *connectTestResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	rw.hijackCalls++
	if rw.conn == nil {
		return nil, nil, io.ErrClosedPipe
	}
	return rw.conn, rw.buffered, nil
}

type connectTestConn struct {
	net.Conn
	failWrites bool
	closed     atomic.Bool
	reads      atomic.Int32
	writes     atomic.Int32
}

func (c *connectTestConn) Read(p []byte) (int, error) {
	c.reads.Add(1)
	return c.Conn.Read(p)
}

func (c *connectTestConn) Write(p []byte) (int, error) {
	c.writes.Add(1)
	if c.failWrites {
		return 0, io.ErrClosedPipe
	}
	return c.Conn.Write(p)
}

func (c *connectTestConn) Close() error {
	c.closed.Store(true)
	return c.Conn.Close()
}

type connectTestBody struct {
	reads  int
	closes int
}

func (b *connectTestBody) Read([]byte) (int, error) {
	b.reads++
	return 0, io.ErrUnexpectedEOF
}

func (b *connectTestBody) Close() error {
	b.closes++
	return nil
}

func TestHTTPServerProtocols(t *testing.T) {
	rp := NewHTTPReverseProxy(HTTPReverseProxyOptions{}, NewRouters())
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	server := &http.Server{
		Handler:           rp,
		ReadHeaderTimeout: time.Second,
		Protocols:         protocols,
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	defer func() {
		require.NoError(t, server.Close())
		require.ErrorIs(t, <-serveErr, http.ErrServerClosed)
	}()

	require.True(t, server.Protocols.HTTP1())
	require.True(t, server.Protocols.UnencryptedHTTP2())

	t.Run("HTTP/1.1", func(t *testing.T) {
		transport := &http.Transport{Protocols: httpProtocols(true, false)}
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport}
		response, err := client.Get("http://" + listener.Addr().String() + "/")
		require.NoError(t, err)
		defer response.Body.Close()

		require.Equal(t, "HTTP/1.1", response.Proto)
		require.Equal(t, http.StatusNotFound, response.StatusCode)
	})

	t.Run("HTTP/2 prior knowledge", func(t *testing.T) {
		transport := &http.Transport{Protocols: httpProtocols(false, true)}
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport}
		response, err := client.Get("http://" + listener.Addr().String() + "/")
		require.NoError(t, err)
		defer response.Body.Close()

		require.Equal(t, "HTTP/2.0", response.Proto)
		require.Equal(t, http.StatusNotFound, response.StatusCode)
	})

	t.Run("HTTP/1.1 Upgrade h2c", func(t *testing.T) {
		conn, err := net.Dial("tcp", listener.Addr().String())
		require.NoError(t, err)
		defer conn.Close()

		_, err = fmt.Fprintf(conn,
			"GET / HTTP/1.1\r\nHost: %s\r\n"+
				"Connection: Upgrade, HTTP2-Settings\r\nUpgrade: h2c\r\n"+
				"HTTP2-Settings: AAMAAABkAAQCAAAAAAIAAAAA\r\n\r\n",
			listener.Addr())
		require.NoError(t, err)
		response, err := http.ReadResponse(bufio.NewReader(conn), nil)
		require.NoError(t, err)
		defer response.Body.Close()

		require.NotEqual(t, http.StatusSwitchingProtocols, response.StatusCode)
	})
}

func httpProtocols(http1, unencryptedHTTP2 bool) *http.Protocols {
	protocols := new(http.Protocols)
	protocols.SetHTTP1(http1)
	protocols.SetUnencryptedHTTP2(unencryptedHTTP2)
	return protocols
}

func TestCheckRouteAuthByRequest(t *testing.T) {
	rc := &RouteConfig{
		Username: "alice",
		Password: "secret",
	}

	t.Run("accepts nil route config", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		require.True(t, checkRouteAuthByRequest(req, nil))
	})

	t.Run("accepts route without credentials", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		require.True(t, checkRouteAuthByRequest(req, &RouteConfig{}))
	})

	t.Run("accepts authorization header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.SetBasicAuth("alice", "secret")
		require.True(t, checkRouteAuthByRequest(req, rc))
	})

	t.Run("accepts proxy authorization header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://target.example.com/", nil)
		req.Header.Set("Proxy-Authorization", httppkg.BasicAuth("alice", "secret"))
		require.True(t, checkRouteAuthByRequest(req, rc))
	})

	t.Run("rejects authorization fallback for proxy request", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://target.example.com/", nil)
		req.SetBasicAuth("alice", "secret")
		require.False(t, checkRouteAuthByRequest(req, rc))
	})

	t.Run("rejects wrong proxy authorization even when authorization matches", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://target.example.com/", nil)
		req.SetBasicAuth("alice", "secret")
		req.Header.Set("Proxy-Authorization", httppkg.BasicAuth("alice", "wrong"))
		require.False(t, checkRouteAuthByRequest(req, rc))
	})

	t.Run("rejects when neither header matches", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://target.example.com/", nil)
		req.SetBasicAuth("alice", "wrong")
		req.Header.Set("Proxy-Authorization", httppkg.BasicAuth("alice", "wrong"))
		require.False(t, checkRouteAuthByRequest(req, rc))
	})

	t.Run("rejects proxy authorization on direct request", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Proxy-Authorization", httppkg.BasicAuth("alice", "secret"))
		require.False(t, checkRouteAuthByRequest(req, rc))
	})
}

func TestGetRequestRouteUser(t *testing.T) {
	t.Run("proxy request uses proxy authorization username", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://target.example.com/", nil)
		req.Host = "target.example.com"
		req.Header.Set("Proxy-Authorization", httppkg.BasicAuth("proxy-user", "proxy-pass"))
		req.SetBasicAuth("direct-user", "direct-pass")

		require.Equal(t, "proxy-user", getRequestRouteUser(req))
	})

	t.Run("connect request keeps proxy authorization routing", func(t *testing.T) {
		req := httptest.NewRequest("CONNECT", "http://target.example.com:443", nil)
		req.Host = "target.example.com:443"
		req.Header.Set("Proxy-Authorization", httppkg.BasicAuth("proxy-user", "proxy-pass"))
		req.SetBasicAuth("direct-user", "direct-pass")

		require.Equal(t, "proxy-user", getRequestRouteUser(req))
	})

	t.Run("direct request uses authorization username", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Host = "example.com"
		req.SetBasicAuth("direct-user", "direct-pass")

		require.Equal(t, "direct-user", getRequestRouteUser(req))
	})

	t.Run("proxy request does not fall back when proxy authorization is invalid", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://target.example.com/", nil)
		req.Host = "target.example.com"
		req.Header.Set("Proxy-Authorization", "Basic !!!")
		req.SetBasicAuth("direct-user", "direct-pass")

		require.Empty(t, getRequestRouteUser(req))
	})
}

func TestCheckRouteAuthByRequestCustomHeader(t *testing.T) {
	rc := &RouteConfig{
		Username:   "alice",
		Password:   "secret",
		AuthHeader: "X-FRP-Auth",
	}

	t.Run("accepts credentials in custom header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-FRP-Auth", httppkg.BasicAuth("alice", "secret"))
		require.True(t, checkRouteAuthByRequest(req, rc))
	})

	t.Run("accepts bare base64 credentials", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-FRP-Auth", strings.TrimPrefix(httppkg.BasicAuth("alice", "secret"), "Basic "))
		require.True(t, checkRouteAuthByRequest(req, rc))
	})

	t.Run("ignores authorization header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.SetBasicAuth("alice", "secret")
		require.False(t, checkRouteAuthByRequest(req, rc))
	})

	t.Run("rejects wrong password in custom header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-FRP-Auth", httppkg.BasicAuth("alice", "wrong"))
		require.False(t, checkRouteAuthByRequest(req, rc))
	})
}

// dialTestVhost starts the reverse proxy on an internal listener and returns the
// client side of a piped connection.
func dialTestVhost(t *testing.T, rp http.Handler) net.Conn {
	t.Helper()

	listener := netpkg.NewInternalListener()
	server := &http.Server{Handler: rp, ReadHeaderTimeout: time.Second}
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	t.Cleanup(func() {
		require.NoError(t, server.Close())
		require.ErrorIs(t, <-serveErr, http.ErrServerClosed)
	})

	client, incoming := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = incoming.Close()
	})
	require.NoError(t, client.SetDeadline(time.Now().Add(5*time.Second)))
	require.NoError(t, listener.PutConn(incoming))
	return client
}

func TestCustomAuthHeaderIsStrippedAndAuthorizationPassesThrough(t *testing.T) {
	remote, backend := net.Pipe()
	defer remote.Close()
	defer backend.Close()
	require.NoError(t, backend.SetDeadline(time.Now().Add(5*time.Second)))

	rp := NewHTTPReverseProxy(HTTPReverseProxyOptions{}, NewRouters())
	require.NoError(t, rp.Register(RouteConfig{
		Domain:     "target.example.com",
		Username:   "alice",
		Password:   "secret",
		AuthHeader: "X-FRP-Auth",
		CreateConnFn: func(string) (net.Conn, error) {
			return remote, nil
		},
	}))

	client := dialTestVhost(t, rp)
	_, err := io.WriteString(client, "GET / HTTP/1.1\r\nHost: target.example.com\r\n"+
		"Authorization: Bearer origin-token\r\n"+
		"X-FRP-Auth: "+httppkg.BasicAuth("alice", "secret")+"\r\n\r\n")
	require.NoError(t, err)

	req, err := http.ReadRequest(bufio.NewReader(backend))
	require.NoError(t, err)
	defer req.Body.Close()
	require.Equal(t, "Bearer origin-token", req.Header.Get("Authorization"))
	require.Empty(t, req.Header.Get("X-FRP-Auth"))

	_, err = io.WriteString(backend, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
	require.NoError(t, err)

	response, err := http.ReadResponse(bufio.NewReader(client), nil)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
}

func TestCustomAuthHeaderDoesNotTriggerBrowserLogin(t *testing.T) {
	rp := NewHTTPReverseProxy(HTTPReverseProxyOptions{}, NewRouters())
	require.NoError(t, rp.Register(RouteConfig{
		Domain:     "target.example.com",
		Username:   "alice",
		Password:   "secret",
		AuthHeader: "X-FRP-Auth",
		CreateConnFn: func(string) (net.Conn, error) {
			t.Fatal("backend must not be reached when auth fails")
			return nil, nil
		},
	}))

	client := dialTestVhost(t, rp)
	// Standard credentials must be ignored so that the origin keeps its Authorization header.
	_, err := io.WriteString(client, "GET / HTTP/1.1\r\nHost: target.example.com\r\n"+
		"Authorization: "+httppkg.BasicAuth("alice", "secret")+"\r\n\r\n")
	require.NoError(t, err)

	response, err := http.ReadResponse(bufio.NewReader(client), nil)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusUnauthorized, response.StatusCode)
	require.Empty(t, response.Header.Get("WWW-Authenticate"))
}

const testLoginPath = defaultAuthPagePath + "/login"

// doRequest sends a raw HTTP request through a fresh test server and returns the
// response with its body already read.
func doRequest(t *testing.T, rp http.Handler, raw string) (*http.Response, string) {
	t.Helper()

	client := dialTestVhost(t, rp)
	_, err := io.WriteString(client, raw)
	require.NoError(t, err)

	resp, err := http.ReadResponse(bufio.NewReader(client), nil)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	return resp, string(body)
}

func postLogin(t *testing.T, rp http.Handler, user, passwd, redirect string) (*http.Response, string) {
	t.Helper()

	form := "username=" + user + "&password=" + passwd + "&redirect=" + redirect
	raw := fmt.Sprintf("POST %s HTTP/1.1\r\nHost: target.example.com\r\nAccept: text/html\r\n"+
		"Content-Type: application/x-www-form-urlencoded\r\nContent-Length: %d\r\n\r\n%s",
		testLoginPath, len(form), form)
	return doRequest(t, rp, raw)
}

func newAuthPageProxy(t *testing.T, createConn CreateConnFunc) *HTTPReverseProxy {
	t.Helper()

	rp := NewHTTPReverseProxy(HTTPReverseProxyOptions{}, NewRouters())
	require.NoError(t, rp.Register(RouteConfig{
		Domain:       "target.example.com",
		Username:     "alice",
		Password:     "secret",
		AuthHeader:   "X-FRP-Auth",
		CreateConnFn: createConn,
	}))
	return rp
}

func TestAuthLoginPageFlow(t *testing.T) {
	remote, backend := net.Pipe()
	defer remote.Close()
	defer backend.Close()
	require.NoError(t, backend.SetDeadline(time.Now().Add(5*time.Second)))

	rp := newAuthPageProxy(t, func(string) (net.Conn, error) { return remote, nil })

	t.Run("browser navigation is redirected to the login page", func(t *testing.T) {
		resp, _ := doRequest(t, rp, "GET /page?a=1 HTTP/1.1\r\nHost: target.example.com\r\nAccept: text/html\r\n\r\n")
		require.Equal(t, http.StatusFound, resp.StatusCode)
		require.Contains(t, resp.Header.Get("Location"), testLoginPath+"?redirect=")
	})

	t.Run("login page renders a form", func(t *testing.T) {
		resp, body := doRequest(t, rp, "GET "+testLoginPath+"?redirect=%2Fpage HTTP/1.1\r\n"+
			"Host: target.example.com\r\nAccept: text/html\r\n\r\n")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Contains(t, body, `name="username"`)
		require.Contains(t, body, `name="password"`)
		require.Contains(t, body, `value="/page"`)
	})

	t.Run("wrong credentials are rejected", func(t *testing.T) {
		resp, body := postLogin(t, rp, "alice", "wrong", "%2Fpage")
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		require.Contains(t, body, "用户名或密码错误")
		require.Empty(t, resp.Cookies())
	})

	t.Run("open redirect is not allowed", func(t *testing.T) {
		resp, _ := postLogin(t, rp, "alice", "secret", "//evil.example.com")
		require.Equal(t, http.StatusFound, resp.StatusCode)
		require.Equal(t, "/", resp.Header.Get("Location"))
	})

	t.Run("correct credentials set a cookie that grants access", func(t *testing.T) {
		resp, _ := postLogin(t, rp, "alice", "secret", "%2Fpage")
		require.Equal(t, http.StatusFound, resp.StatusCode)
		require.Equal(t, "/page", resp.Header.Get("Location"))

		cookies := resp.Cookies()
		require.Len(t, cookies, 1)
		require.True(t, cookies[0].HttpOnly)
		require.Len(t, cookies[0].Name, len("frp_auth_")+16)

		client := dialTestVhost(t, rp)
		_, err := io.WriteString(client, "GET /page HTTP/1.1\r\nHost: target.example.com\r\n"+
			"Accept: text/html\r\nAuthorization: Bearer origin-token\r\n"+
			"Cookie: theme=dark; "+cookies[0].Name+"="+cookies[0].Value+"; lang=zh\r\n\r\n")
		require.NoError(t, err)

		req, err := http.ReadRequest(bufio.NewReader(backend))
		require.NoError(t, err)
		defer req.Body.Close()
		require.Equal(t, "Bearer origin-token", req.Header.Get("Authorization"))
		require.Empty(t, req.Header.Get("X-FRP-Auth"))
		// The frp session cookie is dropped, while the origin's own cookies are kept.
		require.Equal(t, "theme=dark; lang=zh", req.Header.Get("Cookie"))

		_, err = io.WriteString(backend, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
		require.NoError(t, err)

		response, err := http.ReadResponse(bufio.NewReader(client), nil)
		require.NoError(t, err)
		defer response.Body.Close()
		require.Equal(t, http.StatusOK, response.StatusCode)
	})

	t.Run("api clients get 401 without a challenge", func(t *testing.T) {
		resp, _ := doRequest(t, rp, "GET /api HTTP/1.1\r\nHost: target.example.com\r\nAccept: */*\r\n\r\n")
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		require.Empty(t, resp.Header.Get("WWW-Authenticate"))
	})
}

func TestAuthCookieValidation(t *testing.T) {
	rc := &RouteConfig{
		Domain:     "target.example.com",
		Username:   "alice",
		Password:   "secret",
		AuthHeader: "X-FRP-Auth",
	}
	now := time.Now()

	t.Run("round trip", func(t *testing.T) {
		value := rc.signAuthCookie(now.Add(time.Hour).Unix())
		require.True(t, rc.verifyAuthCookie(value, now))
	})

	t.Run("expired cookie is rejected", func(t *testing.T) {
		value := rc.signAuthCookie(now.Add(-time.Minute).Unix())
		require.False(t, rc.verifyAuthCookie(value, now))
	})

	t.Run("tampered signature is rejected", func(t *testing.T) {
		value := rc.signAuthCookie(now.Add(time.Hour).Unix())
		require.False(t, rc.verifyAuthCookie(value+"x", now))
	})

	t.Run("tampered expiry is rejected", func(t *testing.T) {
		value := rc.signAuthCookie(now.Add(time.Hour).Unix())
		parts := strings.Split(value, ".")
		require.Len(t, parts, 3)
		forged := parts[0] + "." + fmt.Sprintf("%d", now.Add(100*time.Hour).Unix()) + "." + parts[2]
		require.False(t, rc.verifyAuthCookie(forged, now))
	})

	t.Run("cookie from another route is rejected", func(t *testing.T) {
		other := *rc
		other.Password = "changed"
		value := rc.signAuthCookie(now.Add(time.Hour).Unix())
		require.False(t, other.verifyAuthCookie(value, now))
	})

	t.Run("plain auth header mode has no login page", func(t *testing.T) {
		defaultHeader := &RouteConfig{Domain: "a", Username: "alice", Password: "secret"}
		require.False(t, defaultHeader.authPageEnabled())
	})
}

func TestAuthPagePath(t *testing.T) {
	rc := &RouteConfig{
		Domain:       "target.example.com",
		Username:     "alice",
		Password:     "secret",
		AuthHeader:   "X-FRP-Auth",
		AuthPagePath: "/_s3cr3t/entry/",
	}

	t.Run("trailing slash is normalized", func(t *testing.T) {
		require.Equal(t, "/_s3cr3t/entry/login", rc.authLoginPath())
		require.Equal(t, "/_s3cr3t/entry/logout", rc.authLogoutPath())
	})

	t.Run("only the configured path is reserved", func(t *testing.T) {
		require.True(t, rc.authPageEnabled())
		require.True(t, rc.isAuthPagePath("/_s3cr3t/entry/login"))
		require.True(t, rc.isAuthPagePath("/_s3cr3t/entry/login/"))
		require.False(t, rc.isAuthPagePath(defaultAuthPagePath+"/login"))
		require.False(t, rc.isAuthPagePath("/"))
	})

	t.Run("empty path falls back to the default", func(t *testing.T) {
		fallback := &RouteConfig{AuthHeader: "X-FRP-Auth", Username: "a", Password: "b"}
		require.Equal(t, defaultAuthPagePath+"/login", fallback.authLoginPath())
	})

	t.Run("browser is redirected to the custom path", func(t *testing.T) {
		rp := NewHTTPReverseProxy(HTTPReverseProxyOptions{}, NewRouters())
		require.NoError(t, rp.Register(*rc))
		resp, _ := doRequest(t, rp, "GET /page HTTP/1.1\r\nHost: target.example.com\r\nAccept: text/html\r\n\r\n")
		require.Equal(t, http.StatusFound, resp.StatusCode)
		require.Equal(t, "/_s3cr3t/entry/login?redirect=%2Fpage", resp.Header.Get("Location"))

		resp, body := doRequest(t, rp, "GET /_s3cr3t/entry/login HTTP/1.1\r\nHost: target.example.com\r\nAccept: text/html\r\n\r\n")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Contains(t, body, `action="/_s3cr3t/entry/login"`)
	})
}

func TestAllowIPsRejectsUntrustedClient(t *testing.T) {
	allowList, err := NewIPAllowList([]string{"192.0.2.0/24"})
	require.NoError(t, err)

	rp := NewHTTPReverseProxy(HTTPReverseProxyOptions{}, NewRouters())
	require.NoError(t, rp.Register(RouteConfig{
		Domain:    "target.example.com",
		Username:  "alice",
		Password:  "secret",
		AllowList: allowList,
		CreateConnFn: func(string) (net.Conn, error) {
			t.Fatal("backend must not be reached when the client IP is rejected")
			return nil, nil
		},
	}))

	t.Run("allowed client reaches the auth check", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Host = "target.example.com"
		req.RemoteAddr = "192.0.2.10:1234"
		rw := httptest.NewRecorder()

		rp.ServeHTTP(rw, req)

		// No credentials, so the request stops at the auth check instead of the IP check.
		require.Equal(t, http.StatusUnauthorized, rw.Code)
	})

	t.Run("other clients are rejected before auth", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Host = "target.example.com"
		req.RemoteAddr = "198.51.100.7:1234"
		req.Header.Set("Authorization", httppkg.BasicAuth("alice", "secret"))
		rw := httptest.NewRecorder()

		rp.ServeHTTP(rw, req)

		require.Equal(t, http.StatusForbidden, rw.Code)
	})
}
