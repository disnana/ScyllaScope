// Package process bounds subprocess output and descendant pipe lifetime.
package process

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

type buffer struct {
	mu        sync.Mutex
	b         bytes.Buffer
	limit     int
	truncated bool
}

func (b *buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	keep := min(n, max(0, b.limit-b.b.Len()))
	_, _ = b.b.Write(p[:keep])
	b.truncated = b.truncated || keep < n
	return n, nil
}
func Output(ctx context.Context, limit int, name string, args ...string) ([]byte, error) {
	return run(ctx, limit, false, name, args...)
}
func CombinedOutput(ctx context.Context, limit int, name string, args ...string) ([]byte, error) {
	return run(ctx, limit, true, name, args...)
}
func run(ctx context.Context, limit int, combined bool, name string, args ...string) ([]byte, error) {
	if limit < 1 {
		return nil, fmt.Errorf("invalid output limit")
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = 250 * time.Millisecond
	b := &buffer{limit: limit}
	cmd.Stdout = b
	cmd.Stderr = io.Discard
	if combined {
		cmd.Stderr = b
	}
	e := cmd.Run()
	if b.truncated {
		return nil, fmt.Errorf("%s output exceeds %d bytes", name, limit)
	}
	return b.b.Bytes(), e
}
