package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/rudolfkova/grpc_auth/pkg/gamekit"

	"client/internal/auth"
	"client/internal/characters"
	"client/internal/config"
	"client/internal/gameclient"
	"client/internal/lobby"
	"client/internal/ws"

	_ "client/internal/tiles" // init: нарезка тайлсетов + те же текстуры, что в редакторе
)

func main() {
	rd := bufio.NewReader(os.Stdin)
	fmt.Print("Gateway (host:port) [localhost:8080]: ")
	gwLine, err := rd.ReadString('\n')
	if err != nil {
		log.Fatal(err)
	}
	gwLine = strings.TrimSpace(strings.TrimSuffix(gwLine, "\n"))
	if gwLine != "" {
		config.SetGatewayHostPort(gwLine)
	}
	log.Printf("gateway %s", config.GatewayHostPort())

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

	characterID, characterName, err := characters.PickTerminal(sess.AccessToken)
	if err != nil {
		log.Fatal(err)
	}
	if characterID == "" {
		log.Println("Персонажи: gateway без character-service — вход в игру без character_id.")
	}

	gameMsgs := make(chan gamekit.Envelope, ws.GameChanCap)
	lobbyPush := make(chan lobby.SubscribeMessage, lobby.SubscribeChanCap)

	gameConn, err := ws.ConnectGame(sess.AccessToken, characterID)
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
	ebiten.SetVsyncEnabled(false)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowSize(gameclient.WindowWidth, gameclient.WindowHeight)
	if err := ebiten.RunGame(gameclient.NewGame(sess.AccessToken, sess.RefreshToken, userID, lobbyID, characterName, chatConn, gameConn, gameMsgs, lobbyPush, lobbyLines)); err != nil {
		log.Fatal(err)
	}
}
