package wswriter

import (
	"github.com/gorilla/websocket"

	"client/internal/gamews"
)

type GameWriter struct {
	conn *websocket.Conn
}

func NewGameWriter(conn *websocket.Conn) *GameWriter {
	return &GameWriter{conn: conn}
}

func (w *GameWriter) Send(typ string, payload any) error {
	return gamews.Send(w.conn, typ, payload)
}
