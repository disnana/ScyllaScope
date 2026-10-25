// Package scyllaclient is the production CQL adapter, using Scylla's gocql
// fork via go.mod replace. Tests elsewhere never substitute a fake module.
package scyllaclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"github.com/gocql/gocql"
	"math"
	"net"
	"os"
	"reflect"
	"scyllascope/internal/core"
	"scyllascope/internal/model"
	"scyllascope/internal/settings"
	"strconv"
	"sync"
	"time"
)

type Client struct {
	cfg     *settings.Config
	mu      sync.Mutex
	session *gocql.Session
	tls     *tls.Config
	closed  bool
}

func TLS(c *settings.Config) (*tls.Config, error) {
	if !c.Scylla.TLSEnabled {
		return nil, nil
	}
	t := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: c.Scylla.TLSServerName}
	if c.Scylla.TLSCA != "" {
		b, e := os.ReadFile(c.Scylla.TLSCA)
		if e != nil {
			return nil, e
		}
		pool, e := x509.SystemCertPool()
		if e != nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(b) {
			return nil, fmt.Errorf("tls_ca contains no PEM certificates")
		}
		t.RootCAs = pool
	}
	return t, nil
}
func New(c *settings.Config) (*Client, error) {
	t, e := TLS(c)
	if e != nil {
		return nil, e
	}
	return &Client{cfg: c, tls: t}, nil
}
func (c *Client) get() (*gocql.Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, fmt.Errorf("CQL client closed")
	}
	if c.session != nil && !c.session.Closed() {
		return c.session, nil
	}
	if len(c.cfg.Scylla.CQLHosts) == 0 {
		return nil, fmt.Errorf("no local CQL host configured")
	}
	// Pin metadata queries to this observer. No silent failover to another
	// observer: the UI must not call a remote node's view its own.
	host := c.cfg.Scylla.CQLHosts[0]
	cfg := gocql.NewCluster(host)
	cfg.Port = c.cfg.Scylla.CQLPort
	cfg.Timeout = c.cfg.Monitoring.Timeout
	cfg.ConnectTimeout = c.cfg.Monitoring.Timeout
	cfg.Consistency = gocql.One
	cfg.NumConns = 1
	cfg.MaxPreparedStmts = 32
	cfg.MaxRoutingKeyInfo = 32
	cfg.PageSize = 32
	cfg.MaxRequestsPerConn = 128
	cfg.MetadataSchemaRequestTimeout = c.cfg.Monitoring.Timeout
	cfg.ProtoVersion = 4
	cfg.DisableInitialHostLookup = true
	cfg.Events.DisableTopologyEvents = true
	cfg.Events.DisableNodeStatusEvents = true
	cfg.Events.DisableSchemaEvents = true
	cfg.HostFilter = gocql.WhiteListHostFilter(host, c.cfg.LocalAddress)
	if c.cfg.Scylla.Username != "" {
		cfg.Authenticator = gocql.PasswordAuthenticator{Username: c.cfg.Scylla.Username, Password: c.cfg.Scylla.Password}
	}
	if c.tls != nil {
		cfg.SslOpts = &gocql.SslOptions{Config: c.tls, EnableHostVerification: true}
	}
	s, e := cfg.CreateSession()
	if e != nil {
		return nil, fmt.Errorf("CQL session %s:%d: %w", host, cfg.Port, e)
	}
	c.session = s
	return s, nil
}
func (c *Client) Close() {
	c.mu.Lock()
	c.closed = true
	s := c.session
	c.session = nil
	c.mu.Unlock()
	if s != nil {
		s.Close()
	}
}
func (c *Client) Discover(ctx context.Context) ([]model.Node, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s, e := c.get()
	if e != nil {
		return nil, e
	}
	it := s.Query(`SELECT * FROM system.cluster_status`).WithContext(ctx).PageSize(128).Iter()
	out := []model.Node{}
	for len(out) <= c.cfg.Monitoring.MaxNodes {
		row := map[string]any{}
		if !it.MapScan(row) {
			break
		}
		n, e := core.ClusterRow(row)
		if e != nil {
			it.Close()
			return nil, e
		}
		out = append(out, n)
	}
	if e = it.Close(); e != nil {
		return nil, fmt.Errorf("system.cluster_status: %w", e)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("system.cluster_status returned no nodes")
	}
	if len(out) > c.cfg.Monitoring.MaxNodes {
		return nil, fmt.Errorf("cluster exceeds max_nodes")
	}
	racks := map[string]string{}
	p := s.Query(`SELECT peer, rack FROM system.peers`).WithContext(ctx).PageSize(32).Iter()
	for count := 0; count <= c.cfg.Monitoring.MaxNodes; count++ {
		row := map[string]any{}
		if !p.MapScan(row) {
			break
		}
		if count == c.cfg.Monitoring.MaxNodes {
			p.Close()
			return nil, fmt.Errorf("system.peers exceeds max_nodes")
		}
		racks[core.Inet(row["peer"])] = core.Text(row["rack"])
	}
	if e = p.Close(); e != nil {
		return nil, fmt.Errorf("system.peers: %w", e)
	}
	var localRack string
	_ = s.Query(`SELECT rack FROM system.local WHERE key='local'`).WithContext(ctx).Scan(&localRack)
	for i := range out {
		if out[i].Rack == "" {
			out[i].Rack = racks[out[i].Address]
		}
		if out[i].Address == c.cfg.LocalAddress && localRack != "" {
			out[i].Rack = localRack
		}
	}
	return out, nil
}
func (c *Client) Query(ctx context.Context, q string, limit int) (model.QueryResult, error) {
	result := model.QueryResult{Columns: []string{}, Rows: []map[string]any{}}
	if e := ctx.Err(); e != nil {
		return result, e
	}
	s, e := c.get()
	if e != nil {
		return result, e
	}
	limit = max(1, min(limit, 500))
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, min(10*time.Second, c.cfg.Monitoring.Timeout*2))
	defer cancel()
	it := s.Query(core.BoundQuery(q, limit)).WithContext(ctx).PageSize(min(limit+1, 32)).Iter()
	for _, col := range it.Columns() {
		result.Columns = append(result.Columns, col.Name)
	}
	resultBudget := 2 << 20
	for len(result.Rows) <= limit {
		row := map[string]any{}
		if !it.MapScan(row) {
			break
		}
		if len(result.Rows) == limit {
			result.Truncated = true
			break
		}
		if e := core.ConsumeValueBudget(row, &resultBudget); e != nil {
			it.Close()
			return result, e
		}
		for k, v := range row {
			row[k] = jsonValue(v)
		}
		result.Rows = append(result.Rows, row)
	}
	if e = it.Close(); e != nil {
		return result, e
	}
	result.Count = len(result.Rows)
	result.TookMS = time.Since(start).Milliseconds()
	return result, nil
}
func jsonValue(v any) any {
	switch x := v.(type) {
	case nil, string, bool:
		return x
	case net.IP:
		return x.String()
	case gocql.UUID:
		return x.String()
	case []byte:
		return "0x" + hex.EncodeToString(x)
	case time.Time:
		return x.Format(time.RFC3339Nano)
	case int64:
		if x > 9007199254740991 || x < -9007199254740991 {
			return strconv.FormatInt(x, 10)
		}
		return x
	case uint64:
		if x > 9007199254740991 {
			return strconv.FormatUint(x, 10)
		}
		return x
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return nil
		}
		return x
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Map:
		m := map[string]any{}
		iter := rv.MapRange()
		for iter.Next() {
			m[fmt.Sprint(iter.Key().Interface())] = jsonValue(iter.Value().Interface())
		}
		return m
	case reflect.Slice, reflect.Array:
		a := []any{}
		for i := 0; i < rv.Len(); i++ {
			a = append(a, jsonValue(rv.Index(i).Interface()))
		}
		return a
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		return rv.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return rv.Uint()
	case reflect.Float32:
		f := rv.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil
		}
		return f
	}
	return fmt.Sprint(v)
}
