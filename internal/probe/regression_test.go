package probe

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestCanceledCQLClosesConnectedSocketPromptly(t *testing.T) {
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	accepted := make(chan struct{})
	closed := make(chan struct{})
	go func() {
		c, e := l.Accept()
		if e != nil {
			return
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(3 * time.Second))
		h := make([]byte, 9)
		io.ReadFull(c, h)
		close(accepted)
		io.Copy(io.Discard, c)
		close(closed)
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan bool, 1)
	go func() { done <- CQL(ctx, "127.0.0.1", l.Addr().(*net.TCPAddr).Port, 3*time.Second, nil).OK }()
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("not connected")
	}
	start := time.Now()
	cancel()
	select {
	case ok := <-done:
		if ok {
			t.Fatal("cancel counted as success")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("canceled query held socket until original timeout")
	}
	select {
	case <-closed:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("socket leaked")
	}
	t.Logf("cancel-to-close=%s", time.Since(start))
}
func TestHTTPTransportHasConnectionAndHeaderBounds(t *testing.T) {
	c := Client(time.Second)
	tr := c.Transport.(*http.Transport)
	defer c.CloseIdleConnections()
	if tr.MaxConnsPerHost != 8 || tr.MaxResponseHeaderBytes != 32<<10 || tr.TLSHandshakeTimeout != time.Second {
		t.Fatal(tr)
	}
}
