package security

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFullExpiredAttemptMapReclaimsCapacity(t *testing.T) {
	g := New(strings.Repeat("k", 64), "127.0.0.1:8091", nil)
	for i := 0; i < 1000; i++ {
		g.attempts[fmt.Sprint(i)] = attempt{at: time.Now().Add(-2 * time.Minute), count: 9}
	}
	r := httptest.NewRequest("POST", "/", nil)
	r.RemoteAddr = "192.0.2.42:1234"
	w := httptest.NewRecorder()
	if !g.Login(w, r, g.Token) {
		t.Fatalf("expired records permanently lock out new hosts: %d", w.Code)
	}
	if len(g.attempts) > 1 {
		t.Fatalf("expired entries retained: %d", len(g.attempts))
	}
}
func TestAttemptCapacityDoesNotAllowBypass(t *testing.T) {
	g := New(strings.Repeat("k", 64), "127.0.0.1:8091", nil)
	for i := 0; i < 1000; i++ {
		g.attempts[fmt.Sprint(i)] = attempt{at: time.Now(), count: 9}
	}
	r := httptest.NewRequest("POST", "/", nil)
	r.RemoteAddr = "192.0.2.42:1234"
	w := httptest.NewRecorder()
	if g.Login(w, r, g.Token) || w.Code != 429 || len(g.attempts) != 1000 {
		t.Fatal("live limit bypassed")
	}
}
