package settings

import (
	"strings"
	"testing"
	"time"
)

func validConfig() *Config {
	c := &Config{NodeID: "x", ClusterName: "x", LocalAddress: "127.0.0.1", Listen: "127.0.0.1:8091"}
	Defaults(c)
	return c
}
func TestBoundsRejectInvalidConfiguration(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{"listen port", func(c *Config) { c.Listen = "127.0.0.1:not-a-port" }},
		{"local IP", func(c *Config) { c.LocalAddress = "not-ip" }},
		{"retry zero", func(c *Config) { c.Notifications.RetryInterval = 0 }},
		{"cache ttl", func(c *Config) { c.Monitoring.KnownNodeRetention = time.Second }},
		{"excess peers", func(c *Config) { c.Peers = make([]Peer, c.Monitoring.MaxNodes+1) }},
		{"public no TLS", func(c *Config) {
			c.Listen = "0.0.0.0:8091"
			c.Security.AllowPublicListen = true
			c.Security.UIToken = strings.Repeat("t", 64)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := validConfig()
			if e := c.Validate(); e != nil {
				t.Fatal("bad fixture", e)
			}
			tc.mutate(c)
			if c.Validate() == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}
