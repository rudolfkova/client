package gamemsg

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/rudolfkova/grpc_auth/pkg/gamekit"
)

type worldStub struct {
	applied []gamekit.Envelope
}

func (w *worldStub) ApplyEnvelope(msg gamekit.Envelope) {
	w.applied = append(w.applied, msg)
}

func TestRouterRoutesStateToWorld(t *testing.T) {
	w := &worldStub{}
	r := NewRouter(w, nil)

	msg := gamekit.Envelope{Service: gamekit.ServiceGame, Type: gamekit.TypeState}
	r.Handle(msg)

	if len(w.applied) != 1 {
		t.Fatalf("expected 1 applied envelope, got %d", len(w.applied))
	}
}

func TestRouterRoutesSaveWorldResultToCallback(t *testing.T) {
	w := &worldStub{}
	called := false
	r := NewRouter(w, func(p gamekit.SaveWorldResultPayload) {
		called = true
		if !p.Ok {
			t.Fatalf("expected ok payload")
		}
	})
	pb, _ := json.Marshal(gamekit.SaveWorldResultPayload{Ok: true})
	msg := gamekit.Envelope{
		Service: gamekit.ServiceGame,
		Type:    gamekit.TypeSaveWorldResult,
		Payload: pb,
	}
	r.Handle(msg)

	if !called {
		t.Fatalf("expected callback call")
	}
	if len(w.applied) != 0 {
		t.Fatalf("save_world_result must not be forwarded to world")
	}
}

func TestDrainClosedChannel(t *testing.T) {
	ch := make(chan gamekit.Envelope)
	close(ch)
	err := Drain(ch, nil)
	if !errors.Is(err, ErrGameMessagesClosed) {
		t.Fatalf("expected ErrGameMessagesClosed, got %v", err)
	}
}
