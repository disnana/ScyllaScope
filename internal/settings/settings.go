// Package settings contains dependency-free configuration types and validation.
package settings

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Peer struct {
	ID      string `yaml:"id" json:"id"`
	Name    string `yaml:"name" json:"name"`
	Address string `yaml:"address" json:"address"`
	API     string `yaml:"api" json:"api"`
}
type Config struct {
	ClusterName     string             `yaml:"cluster_name"`
	NodeID          string             `yaml:"node_id"`
	NodeName        string             `yaml:"node_name"`
	LocalAddress    string             `yaml:"local_address"`
	Listen          string             `yaml:"listen"`
	Token           string             `yaml:"cluster_token"`
	Source          string             `yaml:"-"`
	DetectionIssues []string           `yaml:"-"`
	Scylla          ScyllaConfig       `yaml:"scylla"`
	Peers           []Peer             `yaml:"peers"`
	Monitoring      MonitoringConfig   `yaml:"monitoring"`
	Notifications   NotificationConfig `yaml:"notifications"`
	Storage         StorageConfig      `yaml:"storage"`
	Security        SecurityConfig     `yaml:"security"`
	Console         ConsoleConfig      `yaml:"console"`
}
type ScyllaConfig struct {
	ConfigPath        string   `yaml:"config_path"`
	CQLHosts          []string `yaml:"cql_hosts"`
	CQLPort           int      `yaml:"cql_port"`
	InternodePort     int      `yaml:"internode_port"`
	PrometheusPort    int      `yaml:"prometheus_port"`
	PrometheusAddress string   `yaml:"prometheus_address"`
	RESTURL           string   `yaml:"rest_url"`
	SystemdService    string   `yaml:"systemd_service"`
	Username          string   `yaml:"username"`
	Password          string   `yaml:"password"`
	TLSCA             string   `yaml:"tls_ca"`
	TLSServerName     string   `yaml:"tls_server_name"`
	TLSEnabled        bool     `yaml:"tls_enabled"`
}
type MonitoringConfig struct {
	Interval           time.Duration `yaml:"interval"`
	Timeout            time.Duration `yaml:"timeout"`
	FailureWindow      int           `yaml:"failure_window"`
	RecoveryWindow     int           `yaml:"recovery_window"`
	HistoryHours       int           `yaml:"history_hours"`
	SampleInterval     time.Duration `yaml:"sample_interval"`
	StaleAfter         time.Duration `yaml:"stale_after"`
	ExpectedNodes      int           `yaml:"expected_nodes"`
	Concurrency        int           `yaml:"concurrency"`
	MaxNodes           int           `yaml:"max_nodes"`
	KnownNodeRetention time.Duration `yaml:"known_node_retention"`
}
type NotificationConfig struct {
	AlertmanagerURLs []string      `yaml:"alertmanager_urls"`
	DiscordWebhook   string        `yaml:"discord_webhook"`
	GenericWebhook   string        `yaml:"generic_webhook"`
	SenderNode       string        `yaml:"sender_node"`
	RetryInterval    time.Duration `yaml:"retry_interval"`
}
type StorageConfig struct {
	SQLitePath string `yaml:"sqlite_path"`
}
type SecurityConfig struct {
	UIToken           string   `yaml:"ui_token"`
	UITokenFile       string   `yaml:"ui_token_file"`
	AllowedHosts      []string `yaml:"allowed_hosts"`
	TLSCert           string   `yaml:"tls_cert"`
	TLSKey            string   `yaml:"tls_key"`
	AllowPublicListen bool     `yaml:"allow_public_listen"`
}
type ConsoleConfig struct {
	Enabled         bool `yaml:"enabled"`
	AllowUserTables bool `yaml:"allow_user_tables"`
	MaxRows         int  `yaml:"max_rows"`
}

func Defaults(c *Config) {
	if c.Scylla.ConfigPath == "" {
		c.Scylla.ConfigPath = "/etc/scylla/scylla.yaml"
	}
	if c.Scylla.CQLPort == 0 {
		c.Scylla.CQLPort = 9042
	}
	if c.Scylla.InternodePort == 0 {
		c.Scylla.InternodePort = 7000
	}
	if c.Scylla.PrometheusPort == 0 {
		c.Scylla.PrometheusPort = 9180
	}
	if c.Scylla.SystemdService == "" {
		c.Scylla.SystemdService = "scylla-server"
	}
	if c.Scylla.RESTURL == "" {
		c.Scylla.RESTURL = "http://127.0.0.1:10000"
	}
	if c.Monitoring.Interval == 0 {
		c.Monitoring.Interval = 10 * time.Second
	}
	if c.Monitoring.Timeout == 0 {
		c.Monitoring.Timeout = 3 * time.Second
	}
	if c.Monitoring.FailureWindow == 0 {
		c.Monitoring.FailureWindow = 3
	}
	if c.Monitoring.RecoveryWindow == 0 {
		c.Monitoring.RecoveryWindow = 2
	}
	if c.Monitoring.HistoryHours == 0 {
		c.Monitoring.HistoryHours = 168
	}
	if c.Monitoring.SampleInterval == 0 {
		c.Monitoring.SampleInterval = 30 * time.Second
	}
	if c.Monitoring.StaleAfter == 0 {
		c.Monitoring.StaleAfter = max(30*time.Second, 3*c.Monitoring.Interval)
	}
	if c.Monitoring.Concurrency == 0 {
		c.Monitoring.Concurrency = 4
	}
	if c.Monitoring.KnownNodeRetention == 0 {
		c.Monitoring.KnownNodeRetention = 24 * time.Hour
	}
	if c.Monitoring.MaxNodes == 0 {
		c.Monitoring.MaxNodes = 64
	}
	if c.Notifications.RetryInterval == 0 {
		c.Notifications.RetryInterval = time.Minute
	}
	if c.Storage.SQLitePath == "" {
		c.Storage.SQLitePath = "/var/lib/scyllascope/scyllascope.db"
	}
	if c.Security.UITokenFile == "" {
		c.Security.UITokenFile = "/etc/scyllascope/ui-token"
	}
	if c.Console.MaxRows == 0 {
		c.Console.MaxRows = 200
	}
	if c.NodeName == "" {
		c.NodeName = c.NodeID
	}
	if c.LocalAddress == "" {
		for _, p := range c.Peers {
			if p.ID == c.NodeID {
				c.LocalAddress = p.Address
				break
			}
		}
	}
	if c.LocalAddress == "" {
		c.LocalAddress, _, _ = net.SplitHostPort(c.Listen)
	}
	if c.Scylla.PrometheusAddress == "" {
		c.Scylla.PrometheusAddress = c.LocalAddress
	}
}
func PrivateAddress(host string) bool {
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return host == "localhost"
	}
	_, ts, _ := net.ParseCIDR("100.64.0.0/10")
	return ip.IsLoopback() || ip.IsPrivate() || ts.Contains(ip)
}
func (c *Config) Validate() error {
	if c.NodeID == "" || c.ClusterName == "" {
		return fmt.Errorf("node_id and cluster_name are required")
	}
	h, p, e := net.SplitHostPort(c.Listen)
	if e != nil || p == "" {
		return fmt.Errorf("invalid listen: %q", c.Listen)
	}
	port, portErr := strconv.Atoi(p)
	if portErr != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid listen port")
	}
	if net.ParseIP(c.LocalAddress) == nil {
		return fmt.Errorf("local_address must be an IP")
	}
	if c.Security.AllowPublicListen && (!PrivateAddress(h)) && (c.Security.UIToken == "" || c.Security.TLSCert == "") {
		return fmt.Errorf("public listen requires UI authentication and TLS")
	}
	if !PrivateAddress(h) && !c.Security.AllowPublicListen {
		return fmt.Errorf("listen must bind a loopback/private/Tailscale address; refusing %q (not a public management service)", c.Listen)
	}
	if c.Monitoring.Interval < time.Second || c.Monitoring.Timeout < 100*time.Millisecond || c.Monitoring.Timeout > 30*time.Second {
		return fmt.Errorf("interval must be >=1s, timeout 100ms..30s")
	}
	if c.Monitoring.FailureWindow < 1 || c.Monitoring.FailureWindow > 100 || c.Monitoring.RecoveryWindow < 1 || c.Monitoring.RecoveryWindow > 100 {
		return fmt.Errorf("confirmation windows must be 1..100")
	}
	if c.Monitoring.Concurrency < 1 || c.Monitoring.Concurrency > 16 || c.Monitoring.MaxNodes < 1 || c.Monitoring.MaxNodes > 128 {
		return fmt.Errorf("concurrency 1..16, max_nodes 1..128 required")
	}
	if c.Monitoring.KnownNodeRetention < time.Minute || c.Monitoring.KnownNodeRetention > 30*24*time.Hour {
		return fmt.Errorf("known_node_retention must be 1m..720h")
	}
	if len(c.Peers) > c.Monitoring.MaxNodes {
		return fmt.Errorf("peers exceeds max_nodes")
	}
	if c.Notifications.RetryInterval < time.Second || c.Notifications.RetryInterval > 24*time.Hour {
		return fmt.Errorf("retry_interval must be 1s..24h")
	}
	if c.Monitoring.ExpectedNodes < 0 || c.Monitoring.ExpectedNodes > c.Monitoring.MaxNodes {
		return fmt.Errorf("invalid expected_nodes")
	}
	if c.Monitoring.HistoryHours < 1 || c.Monitoring.HistoryHours > 8760 || c.Monitoring.SampleInterval < 5*time.Second || c.Monitoring.StaleAfter < 2*c.Monitoring.Interval {
		return fmt.Errorf("history_hours 1..8760, sample_interval >=5s, stale_after >=2*interval required")
	}
	for _, p := range []int{c.Scylla.CQLPort, c.Scylla.InternodePort, c.Scylla.PrometheusPort} {
		if p < 1 || p > 65535 {
			return fmt.Errorf("invalid port %d", p)
		}
	}
	if c.Console.MaxRows < 1 || c.Console.MaxRows > 500 {
		return fmt.Errorf("console max_rows must be 1..500")
	}
	if c.Token != "" && len(c.Token) < 32 {
		return fmt.Errorf("cluster_token must contain at least 32 characters")
	}
	if c.Security.UIToken != "" && len(c.Security.UIToken) < 32 {
		return fmt.Errorf("ui_token must contain at least 32 characters")
	}
	if (c.Security.TLSCert == "") != (c.Security.TLSKey == "") {
		return fmt.Errorf("tls_cert and tls_key must be specified together")
	}
	if len(c.Notifications.AlertmanagerURLs) > 8 {
		return fmt.Errorf("at most 8 Alertmanager endpoints")
	}
	if len(c.Notifications.AlertmanagerURLs) > 0 && (c.Notifications.DiscordWebhook != "" || c.Notifications.GenericWebhook != "") {
		return fmt.Errorf("Alertmanager and direct webhooks cannot be combined")
	}
	for _, v := range append([]string{c.Scylla.RESTURL, c.Notifications.DiscordWebhook, c.Notifications.GenericWebhook}, c.Notifications.AlertmanagerURLs...) {
		if v != "" {
			u, e := url.Parse(v)
			if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
				return fmt.Errorf("invalid HTTP URL in configuration")
			}
		}
	}
	for _, p := range c.Peers {
		if net.ParseIP(p.Address) == nil {
			return fmt.Errorf("peer.address must be an IP: %q", p.Address)
		}
		if p.API != "" {
			u, e := url.Parse(p.API)
			if e != nil || u.Hostname() != p.Address || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" {
				return fmt.Errorf("peer API must be http(s) on that peer's address: %q", p.API)
			}
		}
	}
	return nil
}
