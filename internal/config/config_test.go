package config

// Uses the actual YAML library. Tests are part of the network-enabled full build.
import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectReadsRealScyllaYAML(t *testing.T) {
	p := filepath.Join(t.TempDir(), "scylla.yaml")
	os.WriteFile(p, []byte("cluster_name: Example Cluster\nlisten_address: 100.64.0.1\nrpc_address: 100.64.0.1\nnative_transport_port: 9142\nstorage_port: 7100\n"), 0600)
	c, e := detect(p)
	if e != nil {
		t.Fatal(e)
	}
	if c.ClusterName != "Example Cluster" || c.LocalAddress != "100.64.0.1" || c.Scylla.CQLPort != 9142 || c.Scylla.InternodePort != 7100 {
		t.Fatalf("wrong detection %+v", c)
	}
}
func TestDuplicateScyllaKeysFailVisibly(t *testing.T) {
	p := filepath.Join(t.TempDir(), "scylla.yaml")
	os.WriteFile(p, []byte("rpc_address: localhost\nrpc_address: 100.64.0.1\n"), 0600)
	if _, e := detect(p); e == nil {
		t.Fatal("duplicate keys hidden")
	}
}
func TestOverrideDoesNotNeedDuplicateDBConfig(t *testing.T) {
	oldTailscaleIPv4 := tailscaleIPv4Func
	tailscaleIPv4Func = func() string { return "100.64.0.1" }
	t.Cleanup(func() { tailscaleIPv4Func = oldTailscaleIPv4 })

	dir := t.TempDir()
	sc := filepath.Join(dir, "scylla.yaml")
	os.WriteFile(sc, []byte("cluster_name: ActualCluster\nlisten_address: 100.64.0.1\nrpc_address: 100.64.0.1\n"), 0600)
	p := filepath.Join(dir, "scope.yaml")
	os.WriteFile(p, []byte("scylla:\n  config_path: "+sc+"\nnode_name: FriendlyName\n"), 0600)
	c, e := Load(p)
	if e != nil {
		t.Fatal(e)
	}
	if c.ClusterName != "ActualCluster" || c.NodeName != "FriendlyName" {
		t.Fatal(c.ClusterName, c.NodeName)
	}
}
