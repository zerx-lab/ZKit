package audit

import (
	"context"
	"testing"
)

func TestRecordVisibleThroughHolder(t *testing.T) {
	ctx, h := WithHolder(context.Background())
	if h == nil {
		t.Fatal("WithHolder returned nil holder")
	}
	if h.Detail != "" {
		t.Fatalf("fresh holder should be empty, got %q", h.Detail)
	}

	Record(ctx, `{"id":1}`)
	if h.Detail != `{"id":1}` {
		t.Fatalf("Detail = %q, want %q", h.Detail, `{"id":1}`)
	}
}

func TestRecordWithoutHolderIsNoop(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Record panicked on bare ctx: %v", r)
		}
	}()
	Record(context.Background(), "ignored")

	// A context that carries a nil *Holder under the key must also be safe.
	ctx := context.WithValue(context.Background(), holderKey{}, (*Holder)(nil))
	Record(ctx, "ignored")
}

func TestRecordLastWriteWins(t *testing.T) {
	ctx, h := WithHolder(context.Background())
	Record(ctx, "first")
	Record(ctx, "second")
	Record(ctx, "third")
	if h.Detail != "third" {
		t.Fatalf("Detail = %q, want last write %q", h.Detail, "third")
	}
}

func TestHolderSurvivesDerivedContext(t *testing.T) {
	ctx, h := WithHolder(context.Background())
	child, cancel := context.WithCancel(ctx)
	defer cancel()

	Record(child, "from-child")
	if h.Detail != "from-child" {
		t.Fatalf("Detail = %q, want %q", h.Detail, "from-child")
	}
}

func TestHoldersAreIsolatedPerRequest(t *testing.T) {
	ctxA, hA := WithHolder(context.Background())
	ctxB, hB := WithHolder(context.Background())
	Record(ctxA, "a")
	Record(ctxB, "b")
	if hA.Detail != "a" || hB.Detail != "b" {
		t.Fatalf("holders leaked: a=%q b=%q", hA.Detail, hB.Detail)
	}
}
