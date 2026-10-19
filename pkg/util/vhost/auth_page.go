// Copyright 2026 The frp Authors
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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fatedier/frp/pkg/util/log"
	"github.com/fatedier/frp/pkg/util/util"
)

// When httpAuthHeader is configured, browsers have no way to send the credential
// header. frps therefore serves a small login form on a reserved path and keeps
// the authenticated session in a signed cookie.
const (
	// defaultAuthPagePath is configurable per proxy through httpAuthPagePath, so
	// deployments can avoid a path that collides with the local service.
	defaultAuthPagePath = "/.auth"

	authCookieTTL     = 12 * time.Hour
	authCookieVersion = "v1"
)

var loginPageTemplate = template.Must(template.New("frp-login").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>需要登录</title>
<style>
:root{color-scheme:light dark}
body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;background:#f4f3ef;
font-family:-apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif;color:#2c2c2a}
.card{width:320px;padding:28px;border:1px solid #e2e0d8;border-radius:12px;background:#fff}
h1{margin:0 0 6px;font-size:17px;font-weight:500}
.sub{margin:0 0 18px;font-size:13px;color:#5f5e5a}
.err{margin:0 0 14px;padding:8px 10px;border-radius:8px;background:#fcebeb;color:#a32d2d;font-size:13px}
label{display:block;margin-bottom:14px;font-size:13px;color:#5f5e5a}
input{display:block;width:100%;box-sizing:border-box;margin-top:6px;padding:9px 10px;font-size:14px;
border:1px solid #d3d1c7;border-radius:8px;background:#fff;color:#2c2c2a}
input:focus{outline:none;border-color:#185fa5}
button{width:100%;padding:10px;font-size:14px;font-weight:500;color:#fff;background:#185fa5;
border:0;border-radius:8px;cursor:pointer}
.hint{margin:16px 0 0;font-size:12px;line-height:1.6;color:#888780}
@media (prefers-color-scheme:dark){
body{background:#1c1c1a;color:#e6e4dd}
.card{background:#26262a;border-color:#3a3a38}
.sub,label{color:#b4b2a9}
input{background:#1c1c1a;border-color:#4a4a46;color:#e6e4dd}
.hint{color:#888780}
.err{background:#501313;color:#f7c1c1}
}
</style>
</head>
<body>
<main class="card">
<h1>需要登录</h1>
<p class="sub">该站点由 frp 提供访问保护</p>
{{if .Error}}<p class="err">{{.Error}}</p>{{end}}
<form method="post" action="{{.Action}}">
<input type="hidden" name="redirect" value="{{.Redirect}}">
<label>用户名<input name="username" autocomplete="username" autofocus required></label>
<label>密码<input name="password" type="password" autocomplete="current-password" required></label>
<button type="submit">登录</button>
</form>
<p class="hint">账号密码由 frp 校验，不会转发给后端服务；登录状态保存在本域 Cookie 中，12 小时后过期。</p>
</main>
</body>
</html>`))

// authPageEnabled reports whether frps should serve its own login page for this route.
// It only makes sense when a custom auth header is used: with the default header
// the browser can answer the Basic challenge by itself.
func (rc *RouteConfig) authPageEnabled() bool {
	return rc != nil && rc.authEnabled() && rc.AuthHeader != ""
}

// authPageBasePath returns the configured base path without a trailing slash.
func (rc *RouteConfig) authPageBasePath() string {
	if rc == nil || rc.AuthPagePath == "" {
		return defaultAuthPagePath
	}
	base := strings.TrimRight(rc.AuthPagePath, "/")
	if base == "" || base == "/" {
		return defaultAuthPagePath
	}
	return base
}

func (rc *RouteConfig) authLoginPath() string {
	return rc.authPageBasePath() + "/login"
}

func (rc *RouteConfig) authLogoutPath() string {
	return rc.authPageBasePath() + "/logout"
}

func (rc *RouteConfig) isAuthPagePath(path string) bool {
	path = strings.TrimSuffix(path, "/")
	return path == rc.authLoginPath() || path == rc.authLogoutPath()
}

// authCookieName scopes the cookie to a single route, so several proxies sharing
// one domain don't overwrite each other's session.
func (rc *RouteConfig) authCookieName() string {
	sum := sha256.Sum256([]byte(rc.Domain + "\x00" + rc.Location + "\x00" + rc.RouteByHTTPUser))
	return "frp_auth_" + hex.EncodeToString(sum[:8])
}

// authCookieKey derives the signing key from the route credentials, so cookies
// are invalidated automatically when the password changes and no extra secret
// needs to be stored.
func (rc *RouteConfig) authCookieKey() []byte {
	h := sha256.New()
	for _, part := range []string{"frp-vhost-auth", rc.Domain, rc.Location, rc.RouteByHTTPUser, rc.Username, rc.Password} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return h.Sum(nil)
}

func (rc *RouteConfig) signAuthCookie(exp int64) string {
	payload := fmt.Sprintf("%s.%d", authCookieVersion, exp)
	mac := hmac.New(sha256.New, rc.authCookieKey())
	mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (rc *RouteConfig) verifyAuthCookie(value string, now time.Time) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 3 || parts[0] != authCookieVersion {
		return false
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || now.Unix() > exp {
		return false
	}
	mac := hmac.New(sha256.New, rc.authCookieKey())
	mac.Write([]byte(parts[0] + "." + parts[1]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(parts[2]))
}

func (rc *RouteConfig) hasValidAuthCookie(req *http.Request) bool {
	if !rc.authPageEnabled() {
		return false
	}
	cookie, err := req.Cookie(rc.authCookieName())
	if err != nil {
		return false
	}
	return rc.verifyAuthCookie(cookie.Value, time.Now())
}

// stripAuthCookie removes the frp session cookie from a Cookie header while
// keeping any cookie that belongs to the local service.
func stripAuthCookie(header http.Header, name string) {
	values := header.Values("Cookie")
	if len(values) == 0 {
		return
	}

	kept := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ";") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if key, _, found := strings.Cut(part, "="); found && strings.TrimSpace(key) == name {
				continue
			}
			kept = append(kept, part)
		}
	}

	header.Del("Cookie")
	if len(kept) > 0 {
		header.Set("Cookie", strings.Join(kept, "; "))
	}
}

// safeRedirect only allows local absolute paths, so the login page can't be used
// as an open redirect.
func safeRedirect(target string) string {
	if target == "" || !strings.HasPrefix(target, "/") || strings.HasPrefix(target, "//") {
		return "/"
	}
	if strings.ContainsAny(target, "\r\n") {
		return "/"
	}
	return target
}

// wantsHTML reports whether the request comes from a browser navigation, which
// can display a login form, instead of an API client, which expects a status code.
func wantsHTML(req *http.Request) bool {
	return strings.Contains(req.Header.Get("Accept"), "text/html")
}

func (rp *HTTPReverseProxy) serveAuthPage(rw http.ResponseWriter, req *http.Request, rc *RouteConfig) {
	if req.URL.Path == rc.authLogoutPath() {
		http.SetCookie(rw, &http.Cookie{
			Name:     rc.authCookieName(),
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		http.Redirect(rw, req, rc.authLoginPath(), http.StatusFound)
		return
	}

	if req.Method != http.MethodPost {
		writeLoginPage(rw, http.StatusOK, req.URL.Query().Get("redirect"), "", rc.authLoginPath())
		return
	}

	req.Body = http.MaxBytesReader(rw, req.Body, 64<<10)
	if err := req.ParseForm(); err != nil {
		http.Error(rw, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	redirect := safeRedirect(req.PostForm.Get("redirect"))

	user := req.PostForm.Get("username")
	passwd := req.PostForm.Get("password")
	if !util.ConstantTimeEqString(user, rc.Username) || !util.ConstantTimeEqString(passwd, rc.Password) {
		log.Warnf("http auth login failed for host [%s] user [%s] from [%s]", rc.Domain, user, req.RemoteAddr)
		writeLoginPage(rw, http.StatusUnauthorized, redirect, "用户名或密码错误", rc.authLoginPath())
		return
	}

	exp := time.Now().Add(authCookieTTL).Unix()
	http.SetCookie(rw, &http.Cookie{
		Name:     rc.authCookieName(),
		Value:    rc.signAuthCookie(exp),
		Path:     "/",
		MaxAge:   int(authCookieTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	log.Infof("http auth login success for host [%s] user [%s] from [%s]", rc.Domain, user, req.RemoteAddr)
	http.Redirect(rw, req, redirect, http.StatusFound)
}

func writeLoginPage(rw http.ResponseWriter, status int, redirect, errMsg, action string) {
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.Header().Set("Cache-Control", "no-store")
	rw.WriteHeader(status)
	_ = loginPageTemplate.Execute(rw, struct {
		Action   string
		Redirect string
		Error    string
	}{
		Action:   action,
		Redirect: safeRedirect(redirect),
		Error:    errMsg,
	})
}
