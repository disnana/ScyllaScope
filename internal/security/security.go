package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

func Token() (string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return hex.EncodeToString(b), nil
}
func Equal(a, b string) bool {
	x := sha256.Sum256([]byte(a))
	y := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(x[:], y[:]) == 1
}

type Guard struct {
	Token    string
	Hosts    map[string]bool
	mu       sync.Mutex
	attempts map[string]attempt
	sessions map[string]int64
}
type attempt struct {
	at    time.Time
	count int
}

func New(token, listen string, hosts []string) *Guard {
	h, _, _ := net.SplitHostPort(listen)
	m := map[string]bool{strings.ToLower(h): true}
	for _, v := range hosts {
		m[strings.ToLower(v)] = true
	}
	return &Guard{Token: token, Hosts: m, attempts: map[string]attempt{}, sessions: map[string]int64{}}
}
func (g *Guard) HostAllowed(r *http.Request) bool {
	host := r.Host
	if h, _, e := net.SplitHostPort(host); e == nil {
		host = h
	}
	return g.Hosts[strings.ToLower(strings.Trim(host, "[]"))]
}
func (g *Guard) Authenticated(r *http.Request) bool {
	if g.Token == "" {
		return false
	}
	if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") && Equal(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), g.Token) {
		return true
	}
	cookie, e := r.Cookie("scyllascope_session")
	if e != nil {
		return false
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 3 {
		return false
	}
	expires, e := strconv.ParseInt(parts[0], 10, 64)
	if e != nil || time.Now().Unix() >= expires || expires > time.Now().Add(25*time.Hour).Unix() {
		return false
	}
	if !Equal(parts[2], g.sign(parts[0]+"."+parts[1])) {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.sessions[parts[1]] == expires
}
func (g *Guard) sign(payload string) string {
	h := hmac.New(sha256.New, []byte(g.Token))
	h.Write([]byte("ScyllaScope-session-v2:" + payload))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
func (g *Guard) Login(w http.ResponseWriter, r *http.Request, token string) bool {
	g.mu.Lock()
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	a := g.attempts[host]
	if time.Since(a.at) > time.Minute {
		a = attempt{at: time.Now()}
	}
	if a.count < 9 {
		a.count++
	}
	if len(g.attempts) >= 1000 {
		for k, v := range g.attempts {
			if time.Since(v.at) > time.Minute {
				delete(g.attempts, k)
			}
		}
	}
	if _, exists := g.attempts[host]; !exists && len(g.attempts) >= 1000 {
		g.mu.Unlock()
		http.Error(w, "login rate limit reached", 429)
		return false
	}
	g.attempts[host] = a
	g.mu.Unlock()
	if a.count > 8 {
		http.Error(w, "Too many login attempts. Retry after one minute.", 429)
		return false
	}
	if g.Token == "" || !Equal(token, g.Token) {
		http.Error(w, "invalid token", 401)
		return false
	}
	expiry := time.Now().Add(24 * time.Hour)
	nonce, err := Token()
	if err != nil {
		http.Error(w, "session unavailable", 503)
		return false
	}
	g.mu.Lock()
	for k, v := range g.sessions {
		if v <= time.Now().Unix() {
			delete(g.sessions, k)
		}
	}
	if len(g.sessions) >= 256 {
		g.mu.Unlock()
		http.Error(w, "too many active sessions", 429)
		return false
	}
	g.sessions[nonce] = expiry.Unix()
	g.mu.Unlock()
	p := strconv.FormatInt(expiry.Unix(), 10) + "." + nonce
	http.SetCookie(w, &http.Cookie{Name: "scyllascope_session", Value: p + "." + g.sign(p), Path: "/", HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode, Expires: expiry, MaxAge: 86400})
	return true
}

// Revocation is immediate, including existing SSE sessions. Restart also expires sessions.
func (g *Guard) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("scyllascope_session"); err == nil {
		parts := strings.Split(c.Value, ".")
		if len(parts) == 3 {
			g.mu.Lock()
			delete(g.sessions, parts[1])
			g.mu.Unlock()
		}
	}
	http.SetCookie(w, &http.Cookie{Name: "scyllascope_session", Value: "", Path: "/", HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: -1, Expires: time.Unix(1, 0)})
}
func MutationAllowed(r *http.Request) bool {
	if r.Header.Get("X-ScyllaScope-Request") != "1" {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || r.Header.Get("Sec-Fetch-Site") == "same-origin"
	}
	u, e := url.Parse(origin)
	return e == nil && (u.Scheme == "https" || u.Scheme == "http") && strings.EqualFold(u.Host, r.Host)
}
func Headers(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
}
func Bearer(r *http.Request, token string) bool {
	return token != "" && strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") && Equal(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), token)
}
func (g *Guard) String() string {
	return fmt.Sprintf("UI auth configured=%t allowed-hosts=%d", g.Token != "", len(g.Hosts))
}
