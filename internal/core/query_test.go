package core

import (
	"strings"
	"testing"
)

func TestSelectGrammar(t *testing.T) {
	for _, q := range []string{"SELECT * FROM system.local;", "SELECT key, value FROM system.scylla_local WHERE key = 'raft_group0_id';", "SELECT group_id, server_id FROM system.raft_state WHERE group_id = a5406850-b360-11f1-bac9-c41fda905af8;", "SELECT * FROM system_schema.columns WHERE keyspace_name='disnana' AND table_name='settings' LIMIT 10"} {
		if _, e := ValidateQuery(q, false); e != nil {
			t.Errorf("valid %s: %s", q, e)
		}
	}
	for _, q := range []string{"SELECT now() FROM system.local", "SELECT * FROM app.data", "SELECT * FROM system.local ALLOW FILTERING", "SELECT * FROM system.local; DELETE FROM system.local", "SELECT * FROM system.local -- comment", "SELECT * FROM system.local WHERE x = udf()", "SELECT * FROM system.local WHERE x IN (1,2)", "SELECT count(*) FROM system.local", "TRUNCATE system.local", "SELECT * FROM system.local LIMIT -1"} {
		if _, e := ValidateQuery(q, false); e == nil {
			t.Errorf("accepted %s", q)
		}
	}
}
func TestBoundQueryAddsServerSideLimit(t *testing.T) {
	for _, q := range []string{"SELECT * FROM system.local", "SELECT * FROM system.local LIMIT 99999"} {
		v := BoundQuery(q, 200)
		if !strings.HasSuffix(v, "LIMIT 201") {
			t.Fatal(v)
		}
	}
	if v := BoundQuery("SELECT * FROM system.local LIMIT 2", 200); !strings.HasSuffix(v, "LIMIT 2") {
		t.Fatal(v)
	}
}
