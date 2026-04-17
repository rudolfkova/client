package gamemsg

import (
	"encoding/json"
	"errors"
	"log"

	"github.com/rudolfkova/grpc_auth/pkg/gamekit"
)

var ErrGameMessagesClosed = errors.New("game websocket closed")

// EnvelopeApplier принимает envelope и обновляет клиентское состояние.
type EnvelopeApplier interface {
	ApplyEnvelope(msg gamekit.Envelope)
}

type Router struct {
	world            EnvelopeApplier
	onSaveWorldReply func(gamekit.SaveWorldResultPayload)
}

func NewRouter(world EnvelopeApplier, onSaveWorldReply func(gamekit.SaveWorldResultPayload)) *Router {
	return &Router{world: world, onSaveWorldReply: onSaveWorldReply}
}

func (r *Router) Handle(msg gamekit.Envelope) {
	if r.world == nil {
		return
	}
	if msg.Service == gamekit.ServiceGame && msg.Type == gamekit.TypeSaveWorldResult && r.onSaveWorldReply != nil {
		var p gamekit.SaveWorldResultPayload
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			log.Printf("game save_world_result: %v", err)
		} else {
			r.onSaveWorldReply(p)
		}
		return
	}
	r.world.ApplyEnvelope(msg)
}

func Drain(msgs <-chan gamekit.Envelope, handler func(gamekit.Envelope)) error {
	for {
		select {
		case msg, ok := <-msgs:
			if !ok {
				return ErrGameMessagesClosed
			}
			if handler != nil {
				handler(msg)
			}
		default:
			return nil
		}
	}
}
