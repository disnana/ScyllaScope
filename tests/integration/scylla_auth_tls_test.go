package integration_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"scyllascope/internal/scyllaclient"
	"scyllascope/internal/settings"
	"strconv"
	"strings"
	"testing"
	"time"
)

const disposableScyllaImage = "scylladb/scylla@sha256:2366233080ba65622b9ed1b4588709141c4456efdf923809f012b3711b513006"

// This separate opt-in case provisions its own fixture, CA, leaf and credentials.
// All private material stays in t.TempDir and no host trust store is modified.
func TestDisposableScyllaAuthenticationTLS(t *testing.T) {
	if os.Getenv("SCYLLASCOPE_AUTH_TLS_INTEGRATION") != "1" {
		t.Skip("set SCYLLASCOPE_AUTH_TLS_INTEGRATION=1 for disposable authenticated TLS fixture")
	}
	dir := t.TempDir()
	write := func(name string, data []byte) {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	makeCA := func(serial int64) (*x509.Certificate, *rsa.PrivateKey, []byte) {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		template := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: "ScyllaScope disposable fixture CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
		der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
		if err != nil {
			t.Fatal(err)
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}
		return cert, key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	}
	ca, key, caPEM := makeCA(1)
	write("ca.crt", caPEM)
	_, _, wrongCA := makeCA(2)
	write("wrong-ca.crt", wrongCA)
	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(3), Subject: pkix.Name{CommonName: "scylla.fixture.invalid"}, DNSNames: []string{"scylla.fixture.invalid"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, leaf, ca, &leafKey.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	write("server.crt", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	write("server.key", pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(leafKey)}))
	write("scylla.yaml", []byte(`cluster_name: scyllascope-disposable-auth-tls
native_transport_port: 9042
native_shard_aware_transport_port: 19042
authenticator: PasswordAuthenticator
authorizer: CassandraAuthorizer
client_encryption_options:
  enabled: true
  certificate: /run/scyllascope-fixture/server.crt
  keyfile: /run/scyllascope-fixture/server.key
  truststore: /run/scyllascope-fixture/ca.crt
  require_client_auth: false
`))
	docker := func(args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "docker", append([]string{"--host=unix:///var/run/docker.sock"}, args...)...)
		return cmd.CombinedOutput()
	}
	container := fmt.Sprintf("scyllascope-isolated-auth-tls-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		if output, err := docker("rm", "-f", container); err != nil && !strings.Contains(string(output), "No such container") {
			t.Errorf("remove disposable auth/TLS fixture: %v %s", err, output)
		}
	})
	if output, err := docker("run", "--pull=never", "-d", "--name", container, "--label", "scyllascope.integration=isolated-auth-tls", "--memory=768m", "--cpus=1", "--pids-limit=256", "-p", "127.0.0.1::9042", "--mount", "type=bind,src="+dir+",dst=/run/scyllascope-fixture,readonly", disposableScyllaImage, "--smp", "1", "--memory", "384M", "--overprovisioned", "1", "--developer-mode", "1", "--io-setup", "0", "--reactor-backend", "epoll", "--options-file", "/run/scyllascope-fixture/scylla.yaml"); err != nil {
		t.Fatalf("start disposable auth/TLS fixture: %v %s", err, output)
	}
	portsJSON, err := docker("inspect", "--format", "{{json .NetworkSettings.Ports}}", container)
	if err != nil {
		t.Fatal("fixture port inspection failed")
	}
	var ports map[string][]struct {
		HostIP   string `json:"HostIp"`
		HostPort string `json:"HostPort"`
	}
	if err = json.Unmarshal(portsJSON, &ports); err != nil || len(ports["9042/tcp"]) != 1 {
		t.Fatal("unexpected fixture CQL port mapping")
	}
	binding := ports["9042/tcp"][0]
	addr := net.JoinHostPort(binding.HostIP, binding.HostPort)
	if err = validateFixtureEndpoint(addr, portsJSON); err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(binding.HostPort)
	if err != nil {
		t.Fatal(err)
	}
	fixtureIP, err := docker("inspect", "--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", container)
	localAddress := strings.TrimSpace(string(fixtureIP))
	if err != nil || net.ParseIP(localAddress) == nil {
		t.Fatal("fixture network identity unavailable")
	}
	base := &settings.Config{ClusterName: "isolated-auth-tls", NodeID: "fixture", LocalAddress: localAddress}
	settings.Defaults(base)
	base.Scylla.CQLHosts = []string{binding.HostIP}
	base.Scylla.CQLPort = port
	base.Monitoring.Timeout = 2 * time.Second
	base.Scylla.Username = "cassandra"
	base.Scylla.Password = "cassandra" // fixture's documented default, never operational credentials
	base.Scylla.TLSEnabled = true
	base.Scylla.TLSCA = filepath.Join(dir, "ca.crt")
	base.Scylla.TLSServerName = "scylla.fixture.invalid"
	query := func(c *settings.Config) error {
		client, err := scyllaclient.New(c)
		if err != nil {
			return err
		}
		defer client.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		r, err := client.Query(ctx, "SELECT release_version FROM system.local", 1)
		if err == nil && r.Count != 1 {
			return fmt.Errorf("unexpected authenticated fixture result shape")
		}
		return err
	}
	deadline := time.Now().Add(120 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		lastErr = query(base)
		if lastErr == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if lastErr != nil {
		logs, _ := docker("logs", "--tail", "40", container)
		_ = os.WriteFile(filepath.Join(dir, "startup.log"), logs, 0600)
		t.Fatalf("authenticated TLS fixture never became ready: %v (startup diagnostics stayed in private temp dir)", lastErr)
	}
	t.Log("production adapter authenticated successfully over TLS with fixture CA and synthetic hostname")
	tests := []struct {
		name   string
		mutate func(*settings.Config)
	}{
		{"wrong_password_rejected", func(c *settings.Config) { c.Scylla.Password = "synthetic-wrong-password" }},
		{"wrong_CA_rejected", func(c *settings.Config) { c.Scylla.TLSCA = filepath.Join(dir, "wrong-ca.crt") }},
		{"wrong_hostname_rejected", func(c *settings.Config) { c.Scylla.TLSServerName = "wrong.fixture.invalid" }},
		{"missing_password_rejected", func(c *settings.Config) { c.Scylla.Username = ""; c.Scylla.Password = "" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := *base
			tc.mutate(&c)
			if err := query(&c); err == nil {
				t.Fatal("invalid fixture authentication/TLS configuration was accepted")
			}
		})
	}
	// Establish a fresh valid session after every rejection to distinguish a
	// policy rejection from a dead fixture that would reject all connections.
	if err = query(base); err != nil {
		t.Fatal("valid authenticated TLS control failed after rejection cases:", err)
	}
}
