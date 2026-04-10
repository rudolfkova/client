package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/rudolfkova/grpc_auth/pkg/gamekit"

	"client/internal/auth"
	"client/internal/gameclient"
	"client/internal/lobby"
	"client/internal/ws"
)

func main() {
	var email, password string

	fmt.Print("Login...\nemail: ")
	if _, err := fmt.Scanln(&email); err != nil {
		log.Fatal(err)
	}
	fmt.Print("password: ")
	if _, err := fmt.Scanln(&password); err != nil {
		log.Fatal(err)
	}

	sess, err := auth.Login(strings.TrimSpace(email), strings.TrimSpace(password))
	if err != nil {
		log.Fatal(err)
	}

	userID, err := auth.UserIDFromAccessJWT(sess.AccessToken)
	if err != nil {
		log.Fatalf("jwt user_id: %v", err)
	}
	lobbyID, err := lobby.EnsureChat(sess.AccessToken, userID)
	if err != nil {
		log.Fatalf("lobby: %v", err)
	}
	lobbyLines, err := lobby.FetchHistory(sess.AccessToken, lobbyID)
	if err != nil {
		log.Printf("lobby history: %v", err)
		lobbyLines = nil
	}

	gameMsgs := make(chan gamekit.Envelope, ws.GameChanCap)
	lobbyPush := make(chan lobby.SubscribeMessage, lobby.SubscribeChanCap)

	gameConn, err := ws.Connect("/ws/game", sess.AccessToken)
	if err != nil {
		log.Fatalf("ws game dial: %v", err)
	}
	chatConn, err := ws.Connect("/ws/subscribe", sess.AccessToken)
	if err != nil {
		gameConn.Close()
		log.Fatalf("ws subscribe dial: %v", err)
	}
	defer gameConn.Close()
	defer chatConn.Close()

	go ws.RunGameReadPump(gameConn, gameMsgs, "/ws/game")
	go ws.RunSubscribeReadPump(chatConn, lobbyPush, "/ws/subscribe")

	ebiten.SetWindowTitle("dnd game client")
	ebiten.SetVsyncEnabled(true)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowSize(gameclient.WindowWidth, gameclient.WindowHeight)
	if err := ebiten.RunGame(gameclient.NewGame(sess.AccessToken, sess.RefreshToken, userID, lobbyID, chatConn, gameConn, gameMsgs, lobbyPush, lobbyLines)); err != nil {
		log.Fatal(err)
	}
}
