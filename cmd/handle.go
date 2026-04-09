package main

import (
	"encoding/json"
	"log"
)

func (g *Game) handleWSMessage(msg WSMessage) {
	if msg.Service != wsServiceGame {
		return
	}
	switch msg.Type {
	case wsTypeState:
		var p gameStatePayload
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			log.Printf("ws: game state payload: %v", err)
			return
		}
		next := make(map[int]Player, len(p.Players))
		for _, pl := range p.Players {
			next[pl.ID] = pl
		}
		g.players = next
	case wsTypeReject:
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
	case wsTypeError:
		var p struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			log.Printf("ws: server error (unparseable payload): %v raw=%s", err, string(msg.Payload))
			return
		}
		log.Printf("ws: server error message=%q", p.Message)
	default:
		// другие type — по мере появления
	}
}
