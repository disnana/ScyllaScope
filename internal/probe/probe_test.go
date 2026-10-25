package probe

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func supported() []byte {
	b := []byte{0, 1, 0, 11}
	b = append(b, []byte("CQL_VERSION")...)
	b = append(b, 0, 1, 0, 5)
	return append(b, []byte("3.3.1")...)
}
func TestNativeCQLActualTCPFixture(t *testing.T) {
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	go func() {
		c, e := l.Accept()
		if e != nil {
			return
		}
		defer c.Close()
		h := make([]byte, 9)
		if _, e = io.ReadFull(c, h); e != nil {
			return
		}
		if h[4] != 5 {
			return
		}
		b := supported()
		h[0] = 0x84
		h[4] = 6
		binary.BigEndian.PutUint32(h[5:9], uint32(len(b)))
		c.Write(append(h, b...))
	}()
	v := CQL(context.Background(), "127.0.0.1", l.Addr().(*net.TCPAddr).Port, time.Second, nil)
	if !v.OK || v.Kind != "cql_options" {
		t.Fatal(v)
	}
}
func TestPlainTCPListenerNotCQLHealthy(t *testing.T) {
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	defer l.Close()
	go func() {
		c, e := l.Accept()
		if e == nil {
			c.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))
			c.Close()
		}
	}()
	v := CQL(context.Background(), "127.0.0.1", l.Addr().(*net.TCPAddr).Port, time.Second, nil)
	if v.OK {
		t.Fatal(v)
	}
}
func TestCQLDeadline(t *testing.T) {
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	defer l.Close()
	done := make(chan struct{})
	go func() {
		c, e := l.Accept()
		if e == nil {
			defer c.Close()
			<-done
		}
	}()
	now := time.Now()
	v := CQL(context.Background(), "127.0.0.1", l.Addr().(*net.TCPAddr).Port, 100*time.Millisecond, nil)
	close(done)
	if v.OK || time.Since(now) > time.Second {
		t.Fatal(v)
	}
}
func TestHTTPOnly2xxHealthy(t *testing.T) {
	for _, code := range []int{200, 204, 301, 401, 404, 500} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) }))
		v := HTTP(context.Background(), Client(time.Second), srv.URL)
		srv.Close()
		if v.OK != (code >= 200 && code < 300) {
			t.Fatal(code, v)
		}
	}
}
func TestMetricsLabelsTimestampsNaN(t *testing.T) {
	v, e := ParseMetrics(strings.NewReader("# TYPE test gauge\nscylla_compaction_manager_backlog{shard=\"0\",note=\"a b \\\"c\"} 1.5 1234\nscylla_compaction_manager_backlog{shard=\"1\"} 2\nscylla_compaction_manager_compactions NaN\nunknown 9\n"))
	if e != nil || v["scylla_compaction_manager_backlog"] != 3.5 {
		t.Fatalf("%v %v", v, e)
	}
	if _, ok := v["scylla_compaction_manager_compactions"]; ok {
		t.Fatal("NaN fabricated zero")
	}
}
func TestMissingMetricsNotZero(t *testing.T) {
	v, e := ParseMetrics(strings.NewReader("unrelated_metric 0\n"))
	if e != nil || len(v) != 0 {
		t.Fatal(v, e)
	}
}
func TestMalformedSelectedMetricFails(t *testing.T) {
	_, e := ParseMetrics(strings.NewReader("scylla_compaction_manager_backlog{bad 1\n"))
	if e == nil {
		t.Fatal("malformed metric accepted")
	}
}
func TestMetricsHTTP404Failure(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	v, m := Metrics(context.Background(), Client(time.Second), srv.URL)
	if v.OK || m != nil {
		t.Fatal(v, m)
	}
}
