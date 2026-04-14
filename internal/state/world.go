package state

import (
	"encoding/json"
	"log"
	"slices"

	"github.com/rudolfkova/grpc_auth/pkg/gamekit"
)

type tileKey struct {
	x, y, layer int
}

// World — последний снимок из gamekit.TypeState (игроки: id, x, y, hp, face_dx, face_dy).
// Тайлы: map по (x,y,layer) + отсортированный слайс Tiles для отрисовки (см. changelog tile_updates).
type World struct {
	Players map[int64]gamekit.Player
	Tiles   []gamekit.Tile

	tileByKey map[tileKey]gamekit.Tile
}

func NewWorld() *World {
	return &World{
		Players: make(map[int64]gamekit.Player),
	}
}

func (w *World) replaceAllTiles(tiles []gamekit.Tile) {
	if w.tileByKey == nil {
		w.tileByKey = make(map[tileKey]gamekit.Tile, len(tiles))
	} else {
		clear(w.tileByKey)
	}
	for i := range tiles {
		t := tiles[i]
		w.tileByKey[tileKey{t.X, t.Y, t.Layer}] = t
	}
	w.rebuildTilesSlice()
}

func (w *World) rebuildTilesSlice() {
	if len(w.tileByKey) == 0 {
		w.Tiles = nil
		return
	}
	w.Tiles = make([]gamekit.Tile, 0, len(w.tileByKey))
	for _, t := range w.tileByKey {
		w.Tiles = append(w.Tiles, t)
	}
	slices.SortFunc(w.Tiles, func(a, b gamekit.Tile) int {
		if a.Y != b.Y {
			return a.Y - b.Y
		}
		if a.X != b.X {
			return a.X - b.X
		}
		return a.Layer - b.Layer
	})
}

func (w *World) applyTileUpdate(u gamekit.TileUpdate) {
	if w.tileByKey == nil {
		w.tileByKey = make(map[tileKey]gamekit.Tile)
	}
	switch u.Op {
	case gamekit.StateTileUpdateUpsert:
		if u.Tile == nil {
			log.Printf("ws: game state tile_updates upsert without tile, skip")
			return
		}
		t := *u.Tile
		w.tileByKey[tileKey{t.X, t.Y, t.Layer}] = t
	case gamekit.StateTileUpdateRemove:
		delete(w.tileByKey, tileKey{u.X, u.Y, u.Layer})
	default:
		log.Printf("ws: game state tile_updates unknown op %q, skip", u.Op)
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

		if p.Tiles != nil {
			w.replaceAllTiles(*p.Tiles)
		} else if len(p.TileUpdates) > 0 {
			for _, u := range p.TileUpdates {
				w.applyTileUpdate(u)
			}
			w.rebuildTilesSlice()
		}
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
