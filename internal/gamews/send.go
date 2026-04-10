package gamews

import (
	"encoding/json"

	"github.com/gorilla/websocket"
	"github.com/rudolfkova/grpc_auth/pkg/gamekit"
)

func Send(conn *websocket.Conn, typ string, payload any) error {
	pb, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	env := gamekit.Envelope{
		Service: gamekit.ServiceGame,
		Type:    typ,
		Payload: pb,
	}
	raw, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, raw)
}
