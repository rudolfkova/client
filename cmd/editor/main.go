package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/rudolfkova/grpc_auth/pkg/gamekit"

	"client/internal/auth"
	"client/internal/characters"
	"client/internal/editor"
	"client/internal/ws"
)

func main() {
	var email, password string

	fmt.Print("World editor — login\nemail: ")
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

	characterID, _, err := characters.PickTerminal(sess.AccessToken)
	if err != nil {
		log.Fatal(err)
	}
	if characterID == "" {
		log.Println("Персонажи: gateway без character-service — вход в редактор без character_id.")
	}

	gameMsgs := make(chan gamekit.Envelope, ws.GameChanCap)

	gameConn, err := ws.ConnectGame(sess.AccessToken, characterID)
	if err != nil {
		log.Fatalf("ws game dial: %v", err)
	}
	defer gameConn.Close()

	go ws.RunGameReadPump(gameConn, gameMsgs, "/ws/game")

	ebiten.SetWindowTitle("dnd world editor")
	ebiten.SetVsyncEnabled(true)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowSize(editor.WindowWidth, editor.WindowHeight)
	if err := ebiten.RunGame(editor.New(gameConn, gameMsgs)); err != nil {
		log.Fatal(err)
	}
}
