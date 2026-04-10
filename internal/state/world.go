package state

import (
	"encoding/json"
	"log"
	"slices"

	"github.com/rudolfkova/grpc_auth/pkg/gamekit"
)

// World — последний снимок из gamekit.TypeState (игроки: id, x, y, hp, face_dx, face_dy).
type World struct {
	Players map[int64]gamekit.Player
	Tiles   []gamekit.Tile
}

func NewWorld() *World {
	return &World{
		Players: make(map[int64]gamekit.Player),
	}
}

func (w *World) ApplyEnvelope(msg gamekit.Envelope) {
	if msg.Service != gamekit.ServiceGame {
		return
	}
	switch msg.Type {
	case gamekit.TypeState:
		var p gamekit.StatePayload
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			log.Printf("ws: game state payload: %v", err)
			return
		}
		next := make(map[int64]gamekit.Player, len(p.Players))
		for _, pl := range p.Players {
			next[pl.ID] = pl
		}
		w.Players = next
		w.Tiles = slices.Clone(p.Tiles)
	case gamekit.TypeReject:
		var p struct {
			Reason         string `json:"reason"`
			Message        string `json:"message"`
			RequestType    string `json:"request_type"`
			RequestService string `json:"request_service"`
		}
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			log.Printf("ws: request rejected (unparseable payload): %v raw=%s", err, string(msg.Payload))
			return
		}
		log.Printf("ws: request rejected reason=%q message=%q request_type=%q request_service=%q",
			p.Reason, p.Message, p.RequestType, p.RequestService)
	case gamekit.TypeError:
		var p struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			log.Printf("ws: server error (unparseable payload): %v raw=%s", err, string(msg.Payload))
			return
		}
		log.Printf("ws: server error message=%q", p.Message)
	default:
	}
}
