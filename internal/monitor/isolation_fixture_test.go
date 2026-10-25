//go:build memory.counters

package monitor

import (
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"scyllascope/internal/settings"
)

func isolationProbeFixtures(t *testing.T, c *settings.Config) (func(), *atomic.Int64) {
	t.Helper()
	var listeners []net.Listener
	var workers sync.WaitGroup
	var probes, accepted, readFailures, writeFailures, maxStartLag, maxReadDuration atomic.Int64
	cleanup := func() {
		for _, l := range listeners {
			l.Close()
		}
		workers.Wait()
		t.Logf("probe fixture accepted=%d supported=%d nonEOF_read_failures=%d write_failures=%d max_handler_start_lag_ns=%d max_header_read_ns=%d", accepted.Load(), probes.Load(), readFailures.Load(), writeFailures.Load(), maxStartLag.Load(), maxReadDuration.Load())
	}
	port := 0
	for _, ip := range []string{"127.0.0.1", "127.0.0.2", "127.0.0.3"} {
		l, err := net.Listen("tcp", net.JoinHostPort(ip, strconv.Itoa(port)))
		if err != nil {
			cleanup()
			t.Fatal(err)
		}
		listeners = append(listeners, l)
		if port == 0 {
			port = l.Addr().(*net.TCPAddr).Port
		}
		workers.Add(1)
		go func(l net.Listener) {
			defer workers.Done()
			for {
				conn, err := l.Accept()
				if err != nil {
					return
				}
				accepted.Add(1)
				acceptedAt := time.Now()
				workers.Add(1)
				go func() {
					defer workers.Done()
					defer conn.Close()
					lag := time.Since(acceptedAt).Nanoseconds()
					for {
						old := maxStartLag.Load()
						if old >= lag || maxStartLag.CompareAndSwap(old, lag) {
							break
						}
					}
					conn.SetDeadline(time.Now().Add(time.Second))
					header := make([]byte, 9)
					readAt := time.Now()
					_, readErr := io.ReadFull(conn, header)
					readNanos := time.Since(readAt).Nanoseconds()
					for {
						old := maxReadDuration.Load()
						if old >= readNanos || maxReadDuration.CompareAndSwap(old, readNanos) {
							break
						}
					}
					if readErr != nil {
						if readErr != io.EOF {
							readFailures.Add(1)
							t.Logf("fixture header read error: %v", readErr)
						}
						return
					}
					if header[4] != 5 {
						return
					}
					body := append([]byte{0, 1, 0, 11}, []byte("CQL_VERSION")...)
					body = append(body, 0, 1, 0, 5)
					body = append(body, []byte("3.3.1")...)
					header[0] = 0x84
					header[4] = 6
					binary.BigEndian.PutUint32(header[5:9], uint32(len(body)))
					payload := append(header, body...)
					n, writeErr := conn.Write(payload)
					if writeErr != nil || n != len(payload) {
						writeFailures.Add(1)
						t.Logf("fixture supported write bytes=%d expected=%d err=%v", n, len(payload), writeErr)
						return
					}
					probes.Add(1)
				}()
			}
		}(l)
	}
	metrics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "scylla_storage_proxy_coordinator_read_timeouts 0\n")
	}))
	host, p, _ := net.SplitHostPort(strings.TrimPrefix(metrics.URL, "http://"))
	mp, _ := strconv.Atoi(p)
	c.Scylla.CQLPort = port
	c.Scylla.InternodePort = port
	c.Scylla.CQLHosts = []string{"127.0.0.1"}
	c.Scylla.RESTURL = metrics.URL
	c.Scylla.PrometheusAddress = host
	c.Scylla.PrometheusPort = mp
	c.Scylla.SystemdService = "scyllascope-isolation-no-such-service"
	return func() { metrics.Close(); cleanup() }, &probes
}
