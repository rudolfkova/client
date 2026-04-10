package main

import (
	"bytes"
	"fmt"
	"image/color"
	"log"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/gorilla/websocket"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/examples/resources/fonts"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	textv2 "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/mlange-42/ark/ecs"
)

const (
	width  = 1600
	height = 800

	tileSize      = 48
	gridPad       = 40.0
	playerRadius  = 18.0
	labelTextSize = 16.0
	labelAboveGap = 6.0
)

var uiFontSource *textv2.GoTextFaceSource

func init() {
	s, err := textv2.NewGoTextFaceSource(bytes.NewReader(fonts.MPlus1pRegular_ttf))
	if err != nil {
		log.Fatal(err)
	}
	uiFontSource = s
}

const (
	wsServiceGame = "game"
	wsTypeState   = "state"
	wsTypeMove    = "move"
	wsTypeHit     = "hit"
	wsTypeReject  = "reject"
	wsTypeError   = "error"
)

// PlayerPos — позиция персонажа с сервера (payload.state).
type Player struct {
	ID int `json:"id"`
	X  int `json:"x"`
	Y  int `json:"y"`
	HP int `json:"hp"`
}

type gameStatePayload struct {
	Players []Player `json:"players"`
}

type movePayload struct {
	Dx int `json:"dx"`
	Dy int `json:"dy"`
}

type hitPayload struct {
	Target int `json:"target_id"`
	Damage int `json:"damage"`
}

type Game struct {
	world   ecs.World
	jwt     string
	refresh string
	userID  int64

	wsGame *websocket.Conn
	wsChat *websocket.Conn

	wsMsgs      <-chan WSMessage
	wsLobbyPush <-chan LobbyChatMessage

	// players — последний снимок из type=state (ключ — id).
	players map[int]Player

	lastMoveDx, lastMoveDy int

	lobbyLines    []chatLine
	lobbyDraft    string
	lobbyFocused  bool
	lobbyChatSize float64
	lobbyChatID   int64
}

func NewGame(accessToken, refreshToken string, userID int64, lobbyChatID int64, wsChat *websocket.Conn, wsGame *websocket.Conn, wsMsgs <-chan WSMessage, wsLobbyPush <-chan LobbyChatMessage, lobbyLines []chatLine) *Game {
	return &Game{
		jwt:     accessToken,
		refresh: refreshToken,
		userID:  userID,

		wsGame: wsGame,
		wsChat: wsChat,

		wsMsgs:      wsMsgs,
		wsLobbyPush: wsLobbyPush,

		players:       make(map[int]Player),
		lobbyLines:    lobbyLines,
		lobbyChatSize: 13,
		lobbyChatID:   lobbyChatID,
	}
}

func (g *Game) Update() error {
	for {
		select {
		case msg, ok := <-g.wsMsgs:
			if !ok {
				return fmt.Errorf("websocket closed")
			}
			g.handleWSMessage(msg)
		case push, ok := <-g.wsLobbyPush:
			if !ok {
				return fmt.Errorf("lobby websocket closed")
			}
			if push.ChatID != g.lobbyChatID {
				continue
			}
			g.lobbyLines = append(g.lobbyLines, chatLine{id: push.ID, senderID: push.SenderID, text: push.Text})
			if len(g.lobbyLines) > maxLobbyChatLines {
				g.lobbyLines = g.lobbyLines[len(g.lobbyLines)-maxLobbyChatLines:]
			}
		default:
			if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
				g.lobbyFocused = !g.lobbyFocused
			}
			if g.lobbyFocused && ebiten.IsFocused() {
				for _, ch := range ebiten.AppendInputChars(nil) {
					if ch == '\n' || ch == '\r' {
						continue
					}
					if utf8.RuneCountInString(g.lobbyDraft) >= maxLobbyDraftRunes {
						break
					}
					g.lobbyDraft += string(ch)
				}
				if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && g.lobbyDraft != "" {
					_, sz := utf8.DecodeLastRuneInString(g.lobbyDraft)
					g.lobbyDraft = g.lobbyDraft[:len(g.lobbyDraft)-sz]
				}
				if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
					g.sendLobbyLine()
				}
				return nil
			}
			dx, dy := arrowDirection()
			if dx != g.lastMoveDx || dy != g.lastMoveDy {
				g.lastMoveDx, g.lastMoveDy = dx, dy
				if err := g.sendWSEnvelope(wsServiceGame, wsTypeMove, movePayload{Dx: dx, Dy: dy}); err != nil {
					log.Printf("ws write: %v", err)
					return err
				}
			}
			target, damage := hitPlayer()
			if err := g.sendWSEnvelope(wsServiceGame, wsTypeHit, hitPayload{Target: target, Damage: damage}); err != nil {
				log.Printf("ws write: %v", err)
				return err
			}

			return nil
		}
	}
}

func arrowDirection() (dx, dy int) {
	if ebiten.IsKeyPressed(ebiten.KeyArrowLeft) {
		dx--
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowRight) {
		dx++
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowUp) {
		dy--
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowDown) {
		dy++
	}
	return dx, dy
}

func hitPlayer() (target, damage int) {
	if ebiten.IsKeyPressed(ebiten.KeySpace) {
		damage = 1
		target = 4
	}

	return target, damage
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Clear()

	ids := make([]int, 0, len(g.players))
	for id := range g.players {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	face := &textv2.GoTextFace{
		Source: uiFontSource,
		Size:   labelTextSize,
	}

	for _, id := range ids {
		pl := g.players[id]
		cx, cy := worldToScreen(pl.X, pl.Y)
		fill := playerColor(id)

		vector.DrawFilledCircle(screen, cx, cy, playerRadius, fill, true)
		vector.StrokeCircle(screen, cx, cy, playerRadius, 1.5, color.RGBA{0xff, 0xff, 0xff, 0x90}, true)

		label := fmt.Sprintf("%d", pl.HP)
		opts := &textv2.DrawOptions{}
		opts.PrimaryAlign = textv2.AlignCenter
		opts.SecondaryAlign = textv2.AlignEnd
		opts.GeoM.Translate(float64(cx), float64(cy)-playerRadius-labelAboveGap)
		opts.ColorScale.ScaleWithColor(color.RGBA{0xff, 0xff, 0xff, 0xff})
		textv2.Draw(screen, label, face, opts)
	}
	g.drawLobbyChat(screen)
}

func worldToScreen(tileX, tileY int) (cx, cy float32) {
	cx = float32(gridPad) + float32(tileX)*tileSize + tileSize*0.5
	cy = float32(gridPad) + float32(tileY)*tileSize + tileSize*0.5
	return cx, cy
}

func playerColor(id int) color.RGBA {
	// Разные оттенки для разных id (стабильно от значения).
	h := uint8(37*id + 80)
	return color.RGBA{R: h, G: uint8(200 - (id*17)%80), B: uint8(120 + (id*13)%100), A: 0xff}
}

func (g *Game) sendLobbyLine() {
	text := strings.TrimSpace(g.lobbyDraft)
	if text == "" {
		return
	}
	g.lobbyDraft = ""
	if err := postLobbySend(g.jwt, g.lobbyChatID, g.userID, text); err != nil {
		log.Printf("lobby send: %v", err)
	}
}

func (g *Game) drawLobbyChat(screen *ebiten.Image) {
	w, _ := ebiten.WindowSize()
	const panelW float32 = 400
	panelH := float32(height) * 0.42
	x := float32(w) - panelW - 14
	y := float32(14)
	vector.DrawFilledRect(screen, x, y, panelW, panelH, color.RGBA{0x18, 0x18, 0x22, 0xd8}, false)
	vector.StrokeRect(screen, x, y, panelW, panelH, 1, color.RGBA{0x66, 0x66, 0x77, 0xff}, false)

	titleFace := &textv2.GoTextFace{Source: uiFontSource, Size: 14}
	titleOpts := &textv2.DrawOptions{}
	titleOpts.GeoM.Translate(float64(x+10), float64(y+6))
	titleOpts.ColorScale.ScaleWithColor(color.RGBA{0xff, 0xff, 0xff, 0xff})
	textv2.Draw(screen, "Лобби", titleFace, titleOpts)

	face := &textv2.GoTextFace{Source: uiFontSource, Size: g.lobbyChatSize}
	lineStep := float32(g.lobbyChatSize + 3)
	lineY := y + panelH - lineStep - 10
	hint := "Tab — фокус чата"
	if g.lobbyFocused {
		hint = "> " + g.lobbyDraft
	}
	inOpts := &textv2.DrawOptions{}
	inOpts.GeoM.Translate(float64(x+8), float64(lineY))
	inOpts.ColorScale.ScaleWithColor(color.RGBA{0xaa, 0xd5, 0xff, 0xff})
	textv2.Draw(screen, hint, face, inOpts)
	lineY -= lineStep + 6
	msgOpts := &textv2.DrawOptions{}
	msgOpts.ColorScale.ScaleWithColor(color.RGBA{0xcc, 0xcc, 0xcc, 0xff})
	for i := len(g.lobbyLines) - 1; i >= 0 && lineY > y+36; i-- {
		ln := g.lobbyLines[i]
		s := fmt.Sprintf("[%d] %s", ln.senderID, ln.text)
		const maxRunes = 52
		if utf8.RuneCountInString(s) > maxRunes {
			s = string([]rune(s)[:maxRunes]) + "…"
		}
		msgOpts.GeoM.Reset()
		msgOpts.GeoM.Translate(float64(x+8), float64(lineY))
		textv2.Draw(screen, s, face, msgOpts)
		lineY -= lineStep
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

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

	// email = "p2@test.local"
	// password = "123456"

	sess, err := login(strings.TrimSpace(email), strings.TrimSpace(password))
	if err != nil {
		log.Fatal(err)
	}

	userID, err := userIDFromAccessJWT(sess.AccessToken)
	if err != nil {
		log.Fatalf("jwt user_id: %v", err)
	}
	lobbyID, err := ensureLobbyChat(sess.AccessToken, userID)
	if err != nil {
		log.Fatalf("lobby: %v", err)
	}
	lobbyLines, err := fetchLobbyHistory(sess.AccessToken, lobbyID)
	if err != nil {
		log.Printf("lobby history: %v", err)
		lobbyLines = nil
	}

	gameMsgs := make(chan WSMessage, wsMsgChanCap)
	lobbyPush := make(chan LobbyChatMessage, wsMsgChanCap)

	gameConn, err := connectWS("/ws/game", sess.AccessToken)
	if err != nil {
		log.Fatalf("ws game dial: %v", err)
	}
	chatConn, err := connectWS("/ws/subscribe", sess.AccessToken)
	if err != nil {
		gameConn.Close()
		log.Fatalf("ws subscribe dial: %v", err)
	}
	defer gameConn.Close()
	defer chatConn.Close()

	go runWSReadPump(gameConn, gameMsgs, "/ws/game")
	go runSubscribeReadPump(chatConn, lobbyPush, "/ws/subscribe")

	ebiten.SetWindowTitle("dnd client")
	ebiten.SetVsyncEnabled(true)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowSize(width, height)
	if err := ebiten.RunGame(NewGame(sess.AccessToken, sess.RefreshToken, userID, lobbyID, chatConn, gameConn, gameMsgs, lobbyPush, lobbyLines)); err != nil {
		log.Fatal(err)
	}
}
