package main

import (
	"encoding/json"
	"log"
	"net/url"

	"github.com/gorilla/websocket"
)

const (
	gameWSPath   = "/ws/game"
	wsMsgChanCap = 256
)

// WSMessage — конверт от сервера; payload произвольный JSON.
type WSMessage struct {
	Service string          `json:"service"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// func connectGameWS(jwt string) (*websocket.Conn, error) {
// 	u := url.URL{Scheme: "ws", Host: serverAddr, Path: gameWSPath}
// 	q := u.Query()
// 	q.Set("token", jwt)
// 	u.RawQuery = q.Encode()

// 	d := websocket.Dialer{}
// 	conn, resp, err := d.Dial(u.String(), http.Header{})
// 	if err != nil {
// 		if resp != nil {
// 			resp.Body.Close()
// 		}
// 		return nil, err
// 	}
// 	return conn, nil
// }

func connectWS(path, jwt string) (*websocket.Conn, error) {
	u := url.URL{
		Scheme: "ws",
		Host:   serverAddr,
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

func runWSReadPump(conn *websocket.Conn, msgs chan<- WSMessage, path string) {
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
		var m WSMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			log.Printf("ws: skip invalid json: %v", err)
			continue
		}
		msgs <- m
	}
}

func runSubscribeReadPump(conn *websocket.Conn, msgs chan<- LobbyChatMessage, path string) {
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
		var m LobbyChatMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			log.Printf("ws: subscribe skip invalid json: %v", err)
			continue
		}
		msgs <- m
	}
}

func (g *Game) sendWSEnvelope(service, typ string, payload any) error {
	pb, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	env := struct {
		Service string          `json:"service"`
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}{
		Service: service,
		Type:    typ,
		Payload: pb,
	}
	raw, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return g.wsGame.WriteMessage(websocket.TextMessage, raw)
}
