package process

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestOutputHelper(t *testing.T) {
	if os.Getenv("SCYLLASCOPE_TEST_HELPER") != "1" {
		return
	}
	switch os.Args[len(os.Args)-1] {
	case "large":
		fmt.Print(strings.Repeat("x", 1<<20))
	case "hang":
		time.Sleep(5 * time.Second)
	}
	os.Exit(0)
}
func TestCommandOutputIsBounded(t *testing.T) {
	t.Setenv("SCYLLASCOPE_TEST_HELPER", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	b, e := Output(ctx, 128, os.Args[0], "-test.run=^TestOutputHelper$", "large")
	if e == nil || len(b) != 0 || !strings.Contains(e.Error(), "exceeds") {
		t.Fatal(len(b), e)
	}
}
func TestCommandRespectsCancellation(t *testing.T) {
	t.Setenv("SCYLLASCOPE_TEST_HELPER", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, e := Output(ctx, 128, os.Args[0], "-test.run=^TestOutputHelper$", "hang")
	if e == nil || time.Since(start) > time.Second {
		t.Fatal(e, time.Since(start))
	}
}
func TestBoundedBufferReportsAllInputConsumed(t *testing.T) {
	b := &buffer{limit: 10}
	n, e := b.Write([]byte("abcdefghijklmnop"))
	if e != nil || n != 16 || b.b.Len() != 10 || !b.truncated {
		t.Fatal(n, e, b)
	}
}
