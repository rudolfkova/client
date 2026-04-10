package ws

import (
	"encoding/json"
	"log"
	"net/url"

	"github.com/gorilla/websocket"
	"github.com/rudolfkova/grpc_auth/pkg/gamekit"

	"client/internal/config"
	"client/internal/lobby"
)

const GameChanCap = 256

func Connect(path, jwt string) (*websocket.Conn, error) {
	u := url.URL{
		Scheme: "ws",
		Host:   config.GatewayHostPort,
		Path:   path,
	}
	q := u.Query()
	q.Set("token", jwt)
	u.RawQuery = q.Encode()

	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		if resp != nil {
			resp.Body.Close()
		}
		return nil, err
	}
	log.Printf("ws: connected %s", path)
	return conn, nil
}

func RunGameReadPump(conn *websocket.Conn, msgs chan<- gamekit.Envelope, path string) {
	defer close(msgs)
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("ws: disconnected %s (normal close)", path)
			} else {
				log.Printf("ws: disconnected %s: %v", path, err)
			}
			return
		}
		var m gamekit.Envelope
		if err := json.Unmarshal(raw, &m); err != nil {
			log.Printf("ws: skip invalid json: %v", err)
			continue
		}
		msgs <- m
	}
}

func RunSubscribeReadPump(conn *websocket.Conn, msgs chan<- lobby.SubscribeMessage, path string) {
	defer close(msgs)
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("ws: disconnected %s (normal close)", path)
			} else {
				log.Printf("ws: disconnected %s: %v", path, err)
			}
			return
		}
		var m lobby.SubscribeMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			log.Printf("ws: subscribe skip invalid json: %v", err)
			continue
		}
		msgs <- m
	}
}
