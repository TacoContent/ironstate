package conditions

import (
	"testing"

	"github.com/TacoContent/ironstate/internal/filters"
)

func TestTestConditionBasics(t *testing.T) {
	fset := filters.New()
	ctx := map[string]any{"computer_name": "KRAYT", "count": float64(3)}

	ok, err := TestCondition(`computer_name == "KRAYT"`, ctx, fset)
	if err != nil || !ok {
		t.Fatalf("expected true, got %v, err=%v", ok, err)
	}

	ok, err = TestCondition(`count > 5`, ctx, fset)
	if err != nil || ok {
		t.Fatalf("expected false, got %v, err=%v", ok, err)
	}
}

func TestTestWhenEmptyAlwaysPasses(t *testing.T) {
	fset := filters.New()
	ok, err := TestWhen(nil, map[string]any{}, fset)
	if err != nil || !ok {
		t.Fatalf("expected nil When to pass, got %v, err=%v", ok, err)
	}

	ok, err = TestWhen([]any{"", "   "}, map[string]any{}, fset)
	if err != nil || !ok {
		t.Fatalf("expected blank When entries to pass, got %v, err=%v", ok, err)
	}
}

func TestTestWhenListIsAnd(t *testing.T) {
	fset := filters.New()
	ctx := map[string]any{"a": true, "b": false}

	ok, err := TestWhen([]any{"a", "b"}, ctx, fset)
	if err != nil || ok {
		t.Fatalf("expected AND of [true,false] to be false, got %v, err=%v", ok, err)
	}

	ok, err = TestWhen([]any{"a"}, ctx, fset)
	if err != nil || !ok {
		t.Fatalf("expected [true] to pass, got %v, err=%v", ok, err)
	}
}

// TestTestWhenBooleanLiteralGotcha guards the Phase 3 'when'-boolean
// gotcha documented in docs/plans/go-rewrite-progress.md: a raw YAML
// 'when: true'/'when: false' literal must round-trip through
// TestWhen/TestCondition correctly.
func TestTestWhenBooleanLiteralGotcha(t *testing.T) {
	fset := filters.New()

	ok, err := TestWhen([]any{true}, map[string]any{}, fset)
	if err != nil || !ok {
		t.Fatalf("when: true should pass, got %v, err=%v", ok, err)
	}

	ok, err = TestWhen([]any{false}, map[string]any{}, fset)
	if err != nil || ok {
		t.Fatalf("when: false should not pass, got %v, err=%v", ok, err)
	}

	ok, err = TestWhen([]any{true, false}, map[string]any{}, fset)
	if err != nil || ok {
		t.Fatalf("when: [true, false] (AND) should not pass, got %v, err=%v", ok, err)
	}
}

func TestTestConditionInvalidExpressionErrors(t *testing.T) {
	fset := filters.New()
	if _, err := TestCondition("== broken", map[string]any{}, fset); err == nil {
		t.Fatal("expected parse error")
	}
}
