package config

import (
	"context"
	"fmt"
	"gopkg.in/yaml.v3"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"scyllascope/internal/settings"
	"strconv"
	"strings"
	"time"
)

type Config = settings.Config
type Peer = settings.Peer

type scyllaYAML struct {
	ClusterName            string `yaml:"cluster_name"`
	ListenAddress          string `yaml:"listen_address"`
	RPCAddress             string `yaml:"rpc_address"`
	BroadcastAddress       string `yaml:"broadcast_address"`
	BroadcastRPCAddress    string `yaml:"broadcast_rpc_address"`
	StoragePort            int    `yaml:"storage_port"`
	NativeTransportPort    int    `yaml:"native_transport_port"`
	NativeTransportPortSSL int    `yaml:"native_transport_port_ssl"`
	PrometheusPort         int    `yaml:"prometheus_port"`
	PrometheusAddress      string `yaml:"prometheus_address"`
	APIPort                int    `yaml:"api_port"`
	APIAddress             string `yaml:"api_address"`
	ClientEncryption       struct {
		Enabled bool `yaml:"enabled"`
	} `yaml:"client_encryption_options"`
	SeedProvider []struct {
		Parameters []struct {
			Seeds string `yaml:"seeds"`
		} `yaml:"parameters"`
	} `yaml:"seed_provider"`
}

func Load(path string) (*Config, error) {
	var b []byte
	var err error
	scyllaPath := "/etc/scylla/scylla.yaml"
	if path != "" {
		b, err = os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		var pre struct {
			Scylla struct {
				ConfigPath string `yaml:"config_path"`
			} `yaml:"scylla"`
		}
		if len(b) > 0 {
			if err = yaml.Unmarshal(b, &pre); err != nil {
				return nil, fmt.Errorf("parse ScyllaScope config: %w", err)
			}
			if pre.Scylla.ConfigPath != "" {
				scyllaPath = pre.Scylla.ConfigPath
			}
		}
	}
	c, err := detect(scyllaPath)
	if err != nil {
		return nil, err
	}
	c.Source = "auto"
	if len(b) > 0 {
		if err = yaml.Unmarshal(b, c); err != nil {
			return nil, err
		}
		c.Source = path
	}
	settings.Defaults(c)
	if v := os.Getenv("SCYLLASCOPE_CLUSTER_TOKEN"); v != "" {
		c.Token = v
	}
	if v := os.Getenv("SCYLLASCOPE_UI_TOKEN"); v != "" {
		c.Security.UIToken = v
	}
	if v := os.Getenv("SCYLLASCOPE_CQL_PASSWORD"); v != "" {
		c.Scylla.Password = v
	}
	if c.Security.UIToken == "" {
		if token, e := os.ReadFile(c.Security.UITokenFile); e == nil {
			c.Security.UIToken = strings.TrimSpace(string(token))
		} else if !os.IsNotExist(e) {
			return nil, fmt.Errorf("read UI token: %w", e)
		}
	}
	if err = c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}
func Detect() (*Config, error) { return detect("/etc/scylla/scylla.yaml") }
func detect(path string) (*Config, error) {
	c := &Config{}
	c.Scylla.ConfigPath = path
	var sy scyllaYAML
	b, err := os.ReadFile(path)
	if err == nil {
		if e := yaml.Unmarshal(b, &sy); e != nil {
			return nil, fmt.Errorf("invalid Scylla YAML (%s): %w; fix duplicate/invalid fields instead of hiding them", path, e)
		}
	} else if os.IsNotExist(err) {
		c.DetectionIssues = append(c.DetectionIssues, "Scylla設定が見つかりません: "+path)
	} else {
		return nil, err
	}
	host, _ := os.Hostname()
	c.NodeID = sanitizeID(host)
	c.NodeName = host
	c.ClusterName = strings.TrimSpace(sy.ClusterName)
	if c.ClusterName == "" {
		c.ClusterName = "Scylla Cluster"
		c.DetectionIssues = append(c.DetectionIssues, "cluster_name は自動検出できていません")
	}
	ts := tailscaleIPv4Func()
	local := firstUsableIP(sy.BroadcastAddress, sy.ListenAddress)
	if local == "" {
		local = ts
	}
	if local == "" {
		local = "127.0.0.1"
	}
	c.LocalAddress = local
	webIP := ts
	if webIP == "" {
		if settings.PrivateAddress(local) {
			webIP = local
		} else {
			webIP = "127.0.0.1"
		}
	}
	c.Listen = net.JoinHostPort(webIP, "8091")
	c.Security.AllowedHosts = []string{host}
	cql := strings.TrimSpace(sy.RPCAddress)
	if cql == "" || cql == "localhost" || cql == "0.0.0.0" || cql == "::" {
		cql = "127.0.0.1"
	}
	c.Scylla.CQLHosts = []string{cql}
	c.Scylla.CQLPort = sy.NativeTransportPort
	c.Scylla.InternodePort = sy.StoragePort
	c.Scylla.PrometheusPort = sy.PrometheusPort
	c.Scylla.PrometheusAddress = sy.PrometheusAddress
	if c.Scylla.PrometheusAddress == "" {
		c.Scylla.PrometheusAddress = sy.ListenAddress
	}
	if c.Scylla.PrometheusAddress == "0.0.0.0" || c.Scylla.PrometheusAddress == "::" {
		c.Scylla.PrometheusAddress = "127.0.0.1"
	}
	apiHost := strings.TrimSpace(sy.APIAddress)
	if apiHost == "" || apiHost == "0.0.0.0" || apiHost == "::" || apiHost == "localhost" {
		apiHost = "127.0.0.1"
	}
	apiPort := sy.APIPort
	if apiPort == 0 {
		apiPort = 10000
	}
	c.Scylla.RESTURL = "http://" + net.JoinHostPort(apiHost, strconv.Itoa(apiPort))
	if sy.NativeTransportPort == 0 && sy.NativeTransportPortSSL > 0 && sy.ClientEncryption.Enabled {
		c.Scylla.CQLPort = sy.NativeTransportPortSSL
		c.Scylla.TLSEnabled = true
		c.DetectionIssues = append(c.DetectionIssues, "CQL TLSを検出。必要ならscylla.tls_ca / tls_server_nameを設定してください")
	}
	c.Peers = []Peer{{ID: c.NodeID, Name: c.NodeName, Address: local, API: "http://" + c.Listen}}
	seen := map[string]bool{local: true}
	for _, sp := range sy.SeedProvider {
		for _, p := range sp.Parameters {
			for _, seed := range strings.Split(p.Seeds, ",") {
				seed = strings.TrimSpace(seed)
				if seed == "" || seen[seed] || net.ParseIP(seed) == nil || net.ParseIP(seed).IsLoopback() {
					continue
				}
				seen[seed] = true
				c.Peers = append(c.Peers, Peer{ID: seed, Name: seed, Address: seed, API: "http://" + net.JoinHostPort(seed, "8091")})
			}
		}
	}
	settings.Defaults(c)
	return c, nil
}
func WriteOverrideTemplate(path string, c *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	s := fmt.Sprintf(`# ScyllaScope v0.6 overrides; omitted DB settings are read from Scylla YAML.
# All nodes: set the SAME cluster_token for authenticated peer observations.
# Generate once using: openssl rand -hex 32
cluster_token: %q
monitoring:
  interval: %s
  timeout: %s
  failure_window: %d
  recovery_window: %d
  expected_nodes: %d
  sample_interval: %s
  history_hours: %d
console:
  enabled: false
  allow_user_tables: false
notifications:
  discord_webhook: %q
  generic_webhook: %q
# Optionally send only from one observer; otherwise each node sends its own view.
#  sender_node: demo-a
# security:
#   allowed_hosts: [demo-a.example.invalid]
# scylla:
#   username: monitor
#   password: "use SCYLLASCOPE_CQL_PASSWORD instead when possible"
#   tls_enabled: false
`, c.Token, c.Monitoring.Interval, c.Monitoring.Timeout, c.Monitoring.FailureWindow, c.Monitoring.RecoveryWindow, c.Monitoring.ExpectedNodes, c.Monitoring.SampleInterval, c.Monitoring.HistoryHours, c.Notifications.DiscordWebhook, c.Notifications.GenericWebhook)
	return os.WriteFile(path, []byte(s), 0600)
}
func Write(path string, c *Config) error {
	b, e := yaml.Marshal(c)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(path), 0750); e != nil {
		return e
	}
	return os.WriteFile(path, b, 0600)
}

var tailscaleIPv4Func = tailscaleIPv4

func tailscaleIPv4() string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	b, e := exec.CommandContext(ctx, "tailscale", "ip", "-4").Output()
	if e != nil {
		return ""
	}
	for _, v := range strings.Fields(string(b)) {
		if ip := net.ParseIP(v); ip != nil && ip.To4() != nil {
			return ip.String()
		}
	}
	return ""
}
func firstUsableIP(v ...string) string {
	for _, s := range v {
		ip := net.ParseIP(strings.TrimSpace(s))
		if ip != nil && !ip.IsUnspecified() && !ip.IsLoopback() {
			return ip.String()
		}
	}
	return ""
}
func sanitizeID(v string) string {
	v = strings.NewReplacer(" ", "-", "_", "-", ".", "-", ":", "-").Replace(strings.ToLower(strings.TrimSpace(v)))
	if v == "" {
		return "node"
	}
	return v
}
