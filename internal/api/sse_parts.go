package api

import (
	"io"
	"net/http"
	"time"
)

// writeSSEParts keeps one write deadline and one flush for the whole frame.
// JSON is already encoded: pass its bytes directly rather than formatting a
// second payload-sized string. HTTP chunk boundaries are not SSE boundaries.
func writeSSEParts(w http.ResponseWriter, rc *http.ResponseController, header string, payload []byte, terminator string) error {
	_ = rc.SetWriteDeadline(time.Now().Add(5 * time.Second))
	defer rc.SetWriteDeadline(time.Time{})
	if n, err := io.WriteString(w, header); err != nil {
		return err
	} else if n != len(header) {
		return io.ErrShortWrite
	}
	if len(payload) > 0 {
		if n, err := w.Write(payload); err != nil {
			return err
		} else if n != len(payload) {
			return io.ErrShortWrite
		}
	}
	if terminator != "" {
		if n, err := io.WriteString(w, terminator); err != nil {
			return err
		} else if n != len(terminator) {
			return io.ErrShortWrite
		}
	}
	return rc.Flush()
}
