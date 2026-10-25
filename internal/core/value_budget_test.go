package core

import (
	"strings"
	"testing"
	"time"
)

func TestCQLResultBudgetAcceptsSmallRows(t *testing.T) {
	budget := 2 << 20
	for i := 0; i < 500; i++ {
		if e := ConsumeValueBudget(map[string]any{"id": i, "at": time.Time{}, "values": []string{"a", "b"}}, &budget); e != nil {
			t.Fatal(i, e)
		}
	}
}
func TestCQLResultBudgetRejectsLargeCellAndNestedCycle(t *testing.T) {
	cycle := map[string]any{}
	cycle["self"] = cycle
	for _, v := range []any{strings.Repeat("x", 1<<20), make([]byte, 2<<20), make([]int, 100000), cycle} {
		budget := 2 << 20
		if ConsumeValueBudget(v, &budget) == nil {
			t.Fatal("oversized or cyclic value accepted")
		}
	}
}
func TestCQLResultBudgetIsCumulative(t *testing.T) {
	budget := 1024
	v := strings.Repeat("a", 80)
	if ConsumeValueBudget(v, &budget) != nil {
		t.Fatal("first value rejected")
	}
	if ConsumeValueBudget(v, &budget) == nil {
		t.Fatal("second value should exceed shared budget")
	}
}
