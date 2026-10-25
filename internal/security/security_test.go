package security

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTokenEntropyAndEquality(t *testing.T) {
	a, e := Token()
	if e != nil || len(a) != 64 {
		t.Fatal(a, e)
	}
	b, _ := Token()
	if a == b || !Equal(a, a) || Equal(a, b) {
		t.Fatal("tokens")
	}
}
func TestHostRestriction(t *testing.T) {
	g := New(strings.Repeat("a", 64), "100.64.0.1:8091", []string{"scope.example"})
	for host, want := range map[string]bool{"100.64.0.1:8091": true, "scope.example": true, "evil.example": false} {
		r := httptest.NewRequest("GET", "/", nil)
		r.Host = host
		if g.HostAllowed(r) != want {
			t.Fatal(host)
		}
	}
}
func TestCookieFlagsAndForgery(t *testing.T) {
	key := strings.Repeat("a", 64)
	g := New(key, "127.0.0.1:8091", nil)
	r := httptest.NewRequest("POST", "http://127.0.0.1/api/v1/login", nil)
	w := httptest.NewRecorder()
	if !g.Login(w, r, key) {
		t.Fatal(w.Body)
	}
	c := w.Result().Cookies()[0]
	if !c.HttpOnly || int(c.SameSite) != 3 {
		t.Fatal(c)
	}
	r.AddCookie(c)
	if !g.Authenticated(r) {
		t.Fatal("valid cookie rejected")
	}
	c.Value += "x"
	r = httptest.NewRequest("GET", "/", nil)
	r.AddCookie(c)
	if g.Authenticated(r) {
		t.Fatal("forged cookie accepted")
	}
}
func TestCSRFRequiresSameOriginAndHeader(t *testing.T) {
	r := httptest.NewRequest("POST", "http://127.0.0.1:8091/x", nil)
	r.Header.Set("Origin", "http://127.0.0.1:8091")
	if MutationAllowed(r) {
		t.Fatal("missing request header")
	}
	r.Header.Set("X-ScyllaScope-Request", "1")
	if !MutationAllowed(r) {
		t.Fatal("same origin rejected")
	}
	r.Header.Set("Origin", "http://evil.example")
	if MutationAllowed(r) {
		t.Fatal("cross origin accepted")
	}
}
func TestRateLimit(t *testing.T) {
	g := New(strings.Repeat("a", 64), "127.0.0.1:8091", nil)
	for i := 0; i < 9; i++ {
		r := httptest.NewRequest("POST", "/", nil)
		w := httptest.NewRecorder()
		g.Login(w, r, "bad")
		if i == 8 && w.Code != 429 {
			t.Fatal(w.Code)
		}
	}
}

func TestLogoutRevokesOnlyOneSession(t *testing.T) {
	g := New(strings.Repeat("a", 64), "127.0.0.1:8091", nil)
	r1 := httptest.NewRequest("POST", "/", nil)
	r2 := httptest.NewRequest("POST", "/", nil)
	w1, w2 := httptest.NewRecorder(), httptest.NewRecorder()
	g.Login(w1, r1, g.Token)
	g.Login(w2, r2, g.Token)
	r1.AddCookie(w1.Result().Cookies()[0])
	r2.AddCookie(w2.Result().Cookies()[0])
	if !g.Authenticated(r1) || !g.Authenticated(r2) {
		t.Fatal("login")
	}
	g.Logout(httptest.NewRecorder(), r1)
	if g.Authenticated(r1) || !g.Authenticated(r2) {
		t.Fatal("session revocation")
	}
}
