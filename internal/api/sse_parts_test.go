package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

// A deliberately fallible ResponseWriter models buffering wrappers and flush
// failures without needing a socket, timing-sensitive client, or unbounded log.
type frameWriter struct {
	header                            http.Header
	body                              bytes.Buffer
	writes, flushes                   int
	shortAt, failWriteAt, failFlushAt int
	writeErr, flushErr                error
	deadlines                         []time.Time
	writeDeadlineIndices              []int
	flushDeadlineIndex                int
}

func (w *frameWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}
func (*frameWriter) WriteHeader(int) {}
func (w *frameWriter) Write(p []byte) (int, error) {
	w.writes++
	w.writeDeadlineIndices = append(w.writeDeadlineIndices, len(w.deadlines))
	if w.writes == w.failWriteAt {
		return 0, w.writeErr
	}
	if w.writes == w.shortAt {
		n := max(0, len(p)-1)
		w.body.Write(p[:n])
		return n, nil
	}
	return w.body.Write(p)
}
func (w *frameWriter) SetWriteDeadline(deadline time.Time) error {
	w.deadlines = append(w.deadlines, deadline)
	return nil
}
func (w *frameWriter) Flush() { _ = w.FlushError() }
func (w *frameWriter) FlushError() error {
	w.flushes++
	w.flushDeadlineIndex = len(w.deadlines)
	if w.flushes == w.failFlushAt {
		return w.flushErr
	}
	return nil
}

type frameMonitor struct {
	Monitor
	subscribed, unsubscribed int
}

func (m *frameMonitor) Subscribe() chan struct{} {
	m.subscribed++
	return m.Monitor.Subscribe()
}
func (m *frameMonitor) Unsubscribe(ch chan struct{}) {
	m.unsubscribed++
	m.Monitor.Unsubscribe(ch)
}

func TestSSEPartsLargeUnicodeJSONKeepsFramingAndSingleDeadline(t *testing.T) {
	message := strings.Repeat("日本語🙂\nline:\u2028", 20000)
	payload, err := json.Marshal(map[string]string{"message": message})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) < 393216 {
		t.Fatal("fixture did not exercise a large frame")
	}
	w := &frameWriter{}
	header := "id: 123\nevent: snapshot\ndata: "
	if err = writeSSEParts(w, http.NewResponseController(w), header, payload, "\n\n"); err != nil {
		t.Fatal(err)
	}
	want := append(append([]byte(header), payload...), '\n', '\n')
	if !bytes.Equal(w.body.Bytes(), want) {
		t.Fatal("SSE wire bytes changed")
	}
	reader := bufio.NewReader(bytes.NewReader(w.body.Bytes()))
	for _, line := range []string{"id: 123\n", "event: snapshot\n"} {
		got, err := reader.ReadString('\n')
		if err != nil || got != line {
			t.Fatal("SSE header framing", got, err)
		}
	}
	dataLine, err := reader.ReadString('\n')
	if err != nil || !strings.HasPrefix(dataLine, "data: ") {
		t.Fatal("data framing", err)
	}
	var decoded map[string]string
	if err = json.Unmarshal([]byte(strings.TrimSuffix(strings.TrimPrefix(dataLine, "data: "), "\n")), &decoded); err != nil || decoded["message"] != message {
		t.Fatal("JSON/Unicode changed", err)
	}
	if blank, err := reader.ReadString('\n'); err != nil || blank != "\n" {
		t.Fatal("missing frame terminator", err)
	}
	if _, err = reader.ReadByte(); err != io.EOF {
		t.Fatal("trailing frame data", err)
	}
	if w.writes != 3 || w.flushes != 1 || len(w.deadlines) != 2 || w.deadlines[0].IsZero() || !w.deadlines[1].IsZero() || w.flushDeadlineIndex != 1 || !reflect.DeepEqual(w.writeDeadlineIndices, []int{1, 1, 1}) {
		t.Fatal("frame did not share one deadline/flush")
	}
}

func TestSSEPartsShortWriteAndWriteErrorAbortWithoutFlush(t *testing.T) {
	sentinel := errors.New("synthetic write failure")
	for _, kind := range []string{"short", "error"} {
		for part := 1; part <= 3; part++ {
			t.Run(fmt.Sprintf("%s-part-%d", kind, part), func(t *testing.T) {
				w := &frameWriter{writeErr: sentinel}
				want := error(io.ErrShortWrite)
				if kind == "short" {
					w.shortAt = part
				} else {
					w.failWriteAt = part
					want = sentinel
				}
				err := writeSSEParts(w, http.NewResponseController(w), "data: ", []byte(`{"message":"日本語🙂"}`), "\n\n")
				if !errors.Is(err, want) || w.writes != part || w.flushes != 0 || len(w.deadlines) != 2 || !w.deadlines[1].IsZero() {
					t.Fatal("failed frame continued or deadline leaked", err)
				}
			})
		}
	}
}

func TestSSEPartsControlFrameAndFlushFailure(t *testing.T) {
	sentinel := errors.New("synthetic flush failure")
	for _, frame := range []string{"retry: 3000\n\n", ": keepalive\n\n", "event: auth_expired\ndata: {}\n\n"} {
		w := &frameWriter{failFlushAt: 1, flushErr: sentinel}
		err := writeSSEParts(w, http.NewResponseController(w), frame, nil, "")
		if !errors.Is(err, sentinel) || w.body.String() != frame || w.writes != 1 || w.flushes != 1 || len(w.deadlines) != 2 || !w.deadlines[1].IsZero() {
			t.Fatal("control frame or flush error changed", err)
		}
	}
}

func TestStreamLargeSnapshotFlushFailureReleasesSubscriber(t *testing.T) {
	s, m, _ := setupAPI(t)
	observed := &frameMonitor{Monitor: s.mon}
	s.mon = observed
	snapshot := m.Snapshot()
	snapshot.Warnings = []string{strings.Repeat("日本語🙂\n", 50000)}
	m.Publish(snapshot)
	encoded, err := json.Marshal(m.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("synthetic snapshot flush failure")
	w := &frameWriter{failFlushAt: 2, flushErr: sentinel}
	req := httptest.NewRequest("GET", "http://127.0.0.1:8091/api/v1/stream", nil)
	req.Header.Set("Authorization", "Bearer "+s.cfg.Security.UIToken)
	s.Handler().ServeHTTP(w, req)
	want := []byte(fmt.Sprintf("retry: 3000\n\nid: %d\nevent: snapshot\ndata: ", snapshot.GeneratedAt.UnixMilli()))
	want = append(want, encoded...)
	want = append(want, '\n', '\n')
	if !bytes.Equal(w.body.Bytes(), want) || w.flushes != 2 {
		t.Fatal("stream send path framing changed")
	}
	if len(s.sseSlots) != 0 || observed.subscribed != 1 || observed.unsubscribed != 1 {
		t.Fatal("stream slot/subscriber leaked after flush failure")
	}
}

type discardFrameWriter struct{ header http.Header }

func (w *discardFrameWriter) Header() http.Header             { return w.header }
func (*discardFrameWriter) WriteHeader(int)                   {}
func (*discardFrameWriter) Write(p []byte) (int, error)       { return len(p), nil }
func (*discardFrameWriter) WriteString(p string) (int, error) { return len(p), nil }
func (*discardFrameWriter) Flush()                            {}
func (*discardFrameWriter) FlushError() error                 { return nil }
func (*discardFrameWriter) SetWriteDeadline(time.Time) error  { return nil }

// Framing-only allocation comparison: JSON generation and actual sockets are
// deliberately outside this microbenchmark. Do not interpret it as an RSS or
// multi-week leak result. Both branches use identical deadline/flush operations.
func BenchmarkSSELargeSnapshotFraming(b *testing.B) {
	payload, err := json.Marshal(map[string]string{"message": strings.Repeat("日", 131072)})
	if err != nil {
		b.Fatal(err)
	}
	for _, mode := range []string{"formatted", "parts"} {
		b.Run(mode, func(b *testing.B) {
			w := &discardFrameWriter{header: make(http.Header)}
			rc := http.NewResponseController(w)
			b.ReportAllocs()
			b.SetBytes(int64(len(payload)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if mode == "formatted" {
					_ = rc.SetWriteDeadline(time.Now().Add(5 * time.Second))
					_, err = io.WriteString(w, fmt.Sprintf("id: %d\nevent: snapshot\ndata: %s\n\n", 123, payload))
					if err == nil {
						err = rc.Flush()
					}
					_ = rc.SetWriteDeadline(time.Time{})
				} else {
					header := fmt.Sprintf("id: %d\nevent: snapshot\ndata: ", 123)
					err = writeSSEParts(w, rc, header, payload, "\n\n")
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
