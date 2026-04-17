package gamecmd

import (
	"testing"

	"github.com/rudolfkova/grpc_auth/pkg/gamekit"
)

type writerCall struct {
	typ     string
	payload any
}

type writerStub struct {
	calls []writerCall
}

func (w *writerStub) Send(typ string, payload any) error {
	w.calls = append(w.calls, writerCall{typ: typ, payload: payload})
	return nil
}

func TestServiceMoveUsesMoveIntent(t *testing.T) {
	w := &writerStub{}
	svc := NewService(w)

	if err := svc.Move(1, -1); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(w.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(w.calls))
	}
	if w.calls[0].typ != gamekit.TypeMove {
		t.Fatalf("unexpected type: %s", w.calls[0].typ)
	}
	intent, ok := w.calls[0].payload.(gamekit.MoveIntent)
	if !ok {
		t.Fatalf("payload type mismatch: %T", w.calls[0].payload)
	}
	if intent.DX != 1 || intent.DY != -1 {
		t.Fatalf("unexpected payload: %+v", intent)
	}
}

func TestServiceSaveWorldUsesIntent(t *testing.T) {
	w := &writerStub{}
	svc := NewService(w)

	if err := svc.SaveWorld("demo"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(w.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(w.calls))
	}
	if w.calls[0].typ != gamekit.TypeSaveWorld {
		t.Fatalf("unexpected type: %s", w.calls[0].typ)
	}
	intent, ok := w.calls[0].payload.(gamekit.SaveWorldIntent)
	if !ok {
		t.Fatalf("payload type mismatch: %T", w.calls[0].payload)
	}
	if intent.Name != "demo" {
		t.Fatalf("unexpected save world name: %q", intent.Name)
	}
}
