package integration_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"scyllascope/internal/scyllaclient"
	"scyllascope/internal/settings"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Opt-in, disposable local fixture only. Never accepts a remote host or an
// unlabelled container, so accidental production configuration is rejected.
func TestDisposableScyllaCQL(t *testing.T) {
	addr := os.Getenv("SCYLLASCOPE_INTEGRATION_ADDR")
	if addr == "" {
		t.Skip("requires explicitly provisioned disposable local Scylla container")
	}
	host, portText, err := net.SplitHostPort(addr)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		t.Fatal("integration address must be a literal loopback address")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		t.Fatal("invalid fixture port")
	}
	container := os.Getenv("SCYLLASCOPE_INTEGRATION_CONTAINER")
	if !strings.HasPrefix(container, "scyllascope-isolated-") || strings.ContainsAny(container, "/\\\x00\n") {
		t.Fatal("disposable container name required")
	}
	docker := func(args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "docker", append([]string{"--host=unix:///var/run/docker.sock"}, args...)...)
		return cmd.CombinedOutput()
	}
	label, err := docker("inspect", "--format", "{{index .Config.Labels \"scyllascope.integration\"}}", container)
	if err != nil || strings.TrimSpace(string(label)) != "isolated" {
		t.Fatal("fixture must have scyllascope.integration=isolated label")
	}
	// The production adapter deliberately filters host metadata. Accept only
	// the address obtained from this verified disposable container, rather than
	// weakening that filter to accommodate Docker port forwarding.
	fixtureIP, err := docker("inspect", "--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", container)
	localAddress := strings.TrimSpace(string(fixtureIP))
	if err != nil || net.ParseIP(localAddress) == nil {
		t.Fatal("fixture network identity unavailable")
	}
	c := &settings.Config{ClusterName: "isolated-integration", NodeID: "fixture", LocalAddress: localAddress}
	settings.Defaults(c)
	c.Scylla.CQLHosts = []string{host}
	c.Scylla.CQLPort = port
	c.Monitoring.Timeout = 2 * time.Second
	c.Monitoring.MaxNodes = 4
	client, err := scyllaclient.New(c)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	query := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		r, err := client.Query(ctx, "SELECT release_version FROM system.local", 1)
		if err == nil && (r.Count != 1 || len(r.Columns) != 1) {
			return fmt.Errorf("unexpected fixture result shape")
		}
		return err
	}
	waitQuery := func() {
		deadline := time.Now().Add(90 * time.Second)
		for time.Now().Before(deadline) {
			if query() == nil {
				return
			}
			time.Sleep(time.Second)
		}
		t.Fatal("local Scylla did not become CQL-ready")
	}
	waitQuery()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	nodes, err := client.Discover(ctx)
	if err != nil {
		t.Fatal("real system.cluster_status discovery:", err)
	}
	if len(nodes) != 1 {
		t.Fatal("expected exactly one fixture node")
	}
	t.Log("real CQL query and system.cluster_status discovery succeeded")
	canceled, stop := context.WithCancel(context.Background())
	stop()
	if _, err := client.Discover(canceled); err == nil {
		t.Fatal("canceled discovery succeeded")
	}
	if output, err := docker("stop", "--time", "10", container); err != nil {
		t.Fatalf("stop disposable fixture: %v %s", err, output)
	}
	defer docker("start", container)
	start := time.Now()
	if query() == nil {
		t.Fatal("query succeeded against stopped fixture")
	}
	if time.Since(start) > 15*time.Second {
		t.Fatal("stopped fixture query exceeded failure budget")
	}
	if output, err := docker("start", container); err != nil {
		t.Fatalf("restart disposable fixture: %v %s", err, output)
	}
	waitQuery()
	t.Log("existing production adapter recovered after fixture stop/restart")
	client.Close()
	if query() == nil {
		t.Fatal("query after Close succeeded")
	}
}
