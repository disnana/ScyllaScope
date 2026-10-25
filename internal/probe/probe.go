// Package probe performs bounded, read-only health probes. TCP reachability is
// deliberately not presented as proof of replication or Raft health.
package probe

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"scyllascope/internal/model"
	"scyllascope/internal/process"
	"strconv"
	"strings"
	"time"
)

func result(start time.Time, kind string, err error) model.Check {
	c := model.Check{OK: err == nil, State: "ok", Kind: kind, Latency: time.Since(start), CheckedAt: time.Now()}
	if err != nil {
		c.State = "error"
		c.Message = err.Error()
	}
	return c
}
func TCP(ctx context.Context, host string, port int, timeout time.Duration) model.Check {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if conn != nil {
		conn.Close()
	}
	return result(start, "tcp", err)
}

// CQL sends OPTIONS (native protocol v4), expecting SUPPORTED. This checks the
// protocol, not auth/query execution. Session queries are checked separately.
func CQL(ctx context.Context, host string, port int, timeout time.Duration, tlsCfg *tls.Config) model.Check {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return result(start, "cql_options", err)
	}
	defer conn.Close()
	rawConn := conn
	stop := context.AfterFunc(ctx, func() { _ = rawConn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	conn.SetDeadline(deadline)
	if tlsCfg != nil {
		cfg := tlsCfg.Clone()
		if cfg.ServerName == "" {
			cfg.ServerName = host
		}
		tc := tls.Client(conn, cfg)
		if err = tc.HandshakeContext(ctx); err != nil {
			return result(start, "cql_options", err)
		}
		conn = tc
	}
	frame := []byte{0x04, 0, 0, 0, 0x05, 0, 0, 0, 0}
	if _, err = conn.Write(frame); err != nil {
		return result(start, "cql_options", err)
	}
	h := make([]byte, 9)
	if _, err = io.ReadFull(conn, h); err != nil {
		return result(start, "cql_options", err)
	}
	length := binary.BigEndian.Uint32(h[5:9])
	if h[0] != 0x84 || h[1] != 0 || binary.BigEndian.Uint16(h[2:4]) != 0 || h[4] != 0x06 || length > 64*1024 || length < 2 {
		return result(start, "cql_options", fmt.Errorf("unexpected CQL reply (version=%02x opcode=%02x length=%d)", h[0], h[4], length))
	}
	body := make([]byte, length)
	if _, err = io.ReadFull(conn, body); err == nil {
		err = validateSupported(body)
	}
	c := result(start, "cql_options", err)
	if err == nil {
		c.Message = "OPTIONS → SUPPORTED (認証・クエリ実行は別チェック)"
	}
	return c
}
func validateSupported(b []byte) error {
	readString := func() (string, error) {
		if len(b) < 2 {
			return "", io.ErrUnexpectedEOF
		}
		n := int(binary.BigEndian.Uint16(b))
		b = b[2:]
		if len(b) < n {
			return "", io.ErrUnexpectedEOF
		}
		s := string(b[:n])
		b = b[n:]
		return s, nil
	}
	if len(b) < 2 {
		return io.ErrUnexpectedEOF
	}
	n := int(binary.BigEndian.Uint16(b))
	b = b[2:]
	hasCQL := false
	for i := 0; i < n; i++ {
		key, e := readString()
		if e != nil {
			return e
		}
		if len(b) < 2 {
			return io.ErrUnexpectedEOF
		}
		nv := int(binary.BigEndian.Uint16(b))
		b = b[2:]
		for j := 0; j < nv; j++ {
			if _, e = readString(); e != nil {
				return e
			}
		}
		if key == "CQL_VERSION" {
			hasCQL = true
		}
	}
	if !hasCQL || len(b) != 0 {
		return fmt.Errorf("invalid CQL SUPPORTED map")
	}
	return nil
}
func Client(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}).DialContext, MaxIdleConns: 32, MaxIdleConnsPerHost: 2, MaxConnsPerHost: 8, IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: timeout, ResponseHeaderTimeout: timeout, MaxResponseHeaderBytes: 32 << 10}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func HTTP(ctx context.Context, client *http.Client, u string) model.Check {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return result(start, "http", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return result(start, "http", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result(start, "http", fmt.Errorf("HTTP %d", resp.StatusCode))
	}
	c := result(start, "http", nil)
	c.Message = resp.Status
	return c
}
func Service(ctx context.Context, service string, timeout time.Duration) model.Check {
	start := time.Now()
	if service == "disabled" {
		return model.Check{State: "disabled", Kind: "systemd", Message: "systemd check disabled"}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, err := process.CombinedOutput(ctx, 4096, "systemctl", "is-active", service)
	if err != nil || strings.TrimSpace(string(out)) != "active" {
		if err == nil {
			err = fmt.Errorf("%s", strings.TrimSpace(string(out)))
		}
		c := result(start, "systemd", err)
		c.Message = strings.TrimSpace(string(out))
		if c.Message == "" {
			c.Message = err.Error()
		}
		return c
	}
	c := result(start, "systemd", nil)
	c.Message = "active"
	return c
}

var MetricNames = map[string]bool{
	"scylla_compaction_manager_compactions": true, "scylla_compaction_manager_pending_compactions": true, "scylla_compaction_manager_backlog": true, "scylla_database_sstable_read_queue_overloads": true, "scylla_raft_waiter_dropped": true,
	"scylla_storage_proxy_coordinator_read_timeouts": true, "scylla_storage_proxy_coordinator_write_timeouts": true, "scylla_storage_proxy_coordinator_read_unavailable": true, "scylla_storage_proxy_coordinator_write_unavailable": true,
	"scylla_database_total_disk_space_used": true, "scylla_database_live_disk_space_used": true, "scylla_reactor_utilization": true,
}

// ParseMetrics reads selected Prometheus text samples. Missing/non-finite
// samples remain absent. Optional timestamps and spaces inside labels work.
func ParseMetrics(r io.Reader) (map[string]float64, error) {
	out := map[string]float64{}
	s := bufio.NewScanner(io.LimitReader(r, 16*1024*1024+1))
	s.Buffer(make([]byte, 65536), 1024*1024)
	total := 0
	for s.Scan() {
		raw := bytes.TrimSpace(s.Bytes())
		line := raw
		total += len(s.Bytes()) + 1
		if total > 16*1024*1024 {
			return nil, fmt.Errorf("metrics response too large")
		}
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		i := 0
		for i < len(line) && line[i] != '{' && line[i] != ' ' && line[i] != '\t' {
			i++
		}
		name := string(line[:i])
		if !MetricNames[name] {
			continue
		}
		if i < len(line) && line[i] == '{' {
			quoted, escape, closed := false, false, false
			for i++; i < len(line); i++ {
				c := line[i]
				if escape {
					escape = false
					continue
				}
				if quoted && c == '\\' {
					escape = true
					continue
				}
				if c == '"' {
					quoted = !quoted
				}
				if !quoted && c == '}' {
					i++
					closed = true
					break
				}
			}
			if !closed {
				return nil, fmt.Errorf("invalid metrics labels for %s", name)
			}
		}
		fields := strings.Fields(string(line[i:]))
		if len(fields) == 0 {
			return nil, fmt.Errorf("missing metrics value for %s", name)
		}
		v, e := strconv.ParseFloat(fields[0], 64)
		if e != nil {
			return nil, fmt.Errorf("invalid metrics value for %s", name)
		}
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		sum := out[name] + v
		if math.IsInf(sum, 0) || math.IsNaN(sum) {
			return nil, fmt.Errorf("metrics aggregate overflow for %s", name)
		}
		out[name] = sum
	}
	if e := s.Err(); e != nil {
		return nil, e
	}
	return out, nil
}
func Metrics(ctx context.Context, client *http.Client, u string) (model.Check, map[string]float64) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return result(start, "http", err), nil
	}
	req.Header.Set("Accept", "text/plain; version=0.0.4")
	resp, err := client.Do(req)
	if err != nil {
		return result(start, "http", err), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return result(start, "http", fmt.Errorf("HTTP %d", resp.StatusCode)), nil
	}
	v, err := ParseMetrics(resp.Body)
	c := result(start, "http", err)
	if err == nil && len(v) == 0 {
		c.Message = "HTTP OK; supported metric names not present"
	}
	return c, v
}
