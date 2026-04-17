package gameclient

import (
	"fmt"
	"image/color"
	"log"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	textv2 "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/rudolfkova/grpc_auth/pkg/gamekit"

	"client/data"
	"client/internal/gamecontent"
	"client/internal/gamews"
	"client/internal/lobby"
	"client/internal/mapfog"
	"client/internal/playeranim"
	"client/internal/state"
	"client/internal/tiles"
	"client/internal/ui"
	"client/internal/world"
)

const (
	WindowWidth  = 1600
	WindowHeight = 800

	// camFollowLambda — скорость догонки цели (эксп. сглаживание); больше = меньше инерции.
	camFollowLambda = 9.0
	// playerVisLambda — как быстро визуальная позиция догоняет сетку с сервера; больше = меньше «волочения».
	playerVisLambda = 14.0
	// при скачке цели дальше этого (в клетках) — мгновенный snap, без длинного догона
	playerVisSnapTilesSq = 2.25 // 1.5²

	chatBubbleDur   = 5 * time.Second
	chatBubbleRunes = 40

	// playerTileLayer — виртуальный слой персонажей: тайлы с Layer < этого (0=земля, 1=предметы) под ними, Layer ≥ — над (навесы и т.д.).
	playerTileLayer = 2

	statsPanelW      = float32(228)
	statsSlideLambda = 11.0
	statsTitleSize   = 15.0
	statsLineSize    = 13.0
	statsHintSize    = 11.0
)

type chatBubble struct {
	text  string
	until time.Time
}

type Game struct {
	jwt     string
	refresh string
	userID  int64

	wsGame *websocket.Conn
	wsChat *websocket.Conn

	wsMsgs      <-chan gamekit.Envelope
	wsLobbyPush <-chan lobby.SubscribeMessage

	World *state.World

	// Последний отправленный на сервер move (dx,dy); шлём только при смене — без спама 60/s.
	lastSentMoveDx, lastSentMoveDy int
	camX, camY                     float32
	visTile                map[int64]struct{ X, Y float32 }

	lobbyLines    []lobby.Line
	lobbyDraft    string
	lobbyFocused  bool
	lobbyChatSize float64
	lobbyChatID   int64

	chatBubbles map[int64]chatBubble // sender_id == id игрока в игре

	demoWalkPhase  map[int64]float64
	demoWalkMoving map[int64]bool

	characterDisplayName string
	statsPanelOpen       bool
	statsSlide           float32 // 0 = спрятано влево, 1 = видно (сглаживание)

	interactTextures map[string]struct{} // texture == id из catalog с interact
	pickableTextures map[string]struct{} // texture == id из catalog с pickable (подбор СКМ)

	worldFog *mapfog.Fog // туман в клетках без слоя 0 (после всех тайлов, «слой 10»)
	fogMode  mapfog.RenderMode // F4: туман+блюр → только туман → только блюр → выкл

	// animTilesStart — фаза anim/* тайлсетов (как в редакторе: tiles.SetEditorAnimTime).
	animTilesStart time.Time

	// invPickSlot — первый слот для обмена (inventory_move); пусто = нет выбора.
	invPickSlot string
}

func NewGame(accessToken, refreshToken string, userID int64, lobbyChatID int64, characterDisplayName string, wsChat *websocket.Conn, wsGame *websocket.Conn, wsMsgs <-chan gamekit.Envelope, wsLobbyPush <-chan lobby.SubscribeMessage, lobbyLines []lobby.Line) *Game {
	_ = ui.FontSource()
	return &Game{
		jwt:     accessToken,
		refresh: refreshToken,
		userID:  userID,

		wsGame: wsGame,
		wsChat: wsChat,

		wsMsgs:      wsMsgs,
		wsLobbyPush: wsLobbyPush,

		World:          state.NewWorld(),
		visTile:        make(map[int64]struct{ X, Y float32 }),
		lobbyLines:     lobbyLines,
		lobbyChatSize:  13,
		lobbyChatID:    lobbyChatID,
		chatBubbles:    make(map[int64]chatBubble),
		demoWalkPhase:  make(map[int64]float64),
		demoWalkMoving: make(map[int64]bool),

		characterDisplayName: strings.TrimSpace(characterDisplayName),
		statsPanelOpen:       true,
		statsSlide:           1,

		interactTextures: gamecontent.InteractTextureSet(data.ContentCatalogJSON),
		pickableTextures: gamecontent.PickableTextureSet(data.ContentCatalogJSON),

		worldFog:       mapfog.NewFog(),
		animTilesStart: time.Now(),
	}
}

func (g *Game) Update() error {
	tiles.SetEditorAnimTime(time.Since(g.animTilesStart).Seconds())
	g.tickStatsPanelSlide()
	for {
		select {
		case msg, ok := <-g.wsMsgs:
			if !ok {
				return fmt.Errorf("websocket closed")
			}
			g.World.ApplyEnvelope(msg)
		case push, ok := <-g.wsLobbyPush:
			if !ok {
				return fmt.Errorf("lobby websocket closed")
			}
			if push.ChatID != g.lobbyChatID {
				continue
			}
			g.lobbyLines = append(g.lobbyLines, lobby.Line{ID: push.ID, SenderID: push.SenderID, Text: push.Text})
			if len(g.lobbyLines) > lobby.MaxChatLines {
				g.lobbyLines = g.lobbyLines[len(g.lobbyLines)-lobby.MaxChatLines:]
			}
			bt := strings.TrimSpace(strings.ReplaceAll(push.Text, "\n", " "))
			if utf8.RuneCountInString(bt) > chatBubbleRunes {
				bt = string([]rune(bt)[:chatBubbleRunes]) + "…"
			}
			g.chatBubbles[push.SenderID] = chatBubble{text: bt, until: time.Now().Add(chatBubbleDur)}
		default:
			if inpututil.IsKeyJustPressed(ebiten.KeyF4) {
				g.fogMode = mapfog.NextRenderMode(g.fogMode)
			}
			if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
				g.lobbyFocused = !g.lobbyFocused
			}
			if g.lobbyFocused && ebiten.IsFocused() {
				for _, ch := range ebiten.AppendInputChars(nil) {
					if ch == '\n' || ch == '\r' {
						continue
					}
					if utf8.RuneCountInString(g.lobbyDraft) >= lobby.MaxDraftRunes {
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
			} else {
				if inpututil.IsKeyJustPressed(ebiten.KeyC) {
					g.statsPanelOpen = !g.statsPanelOpen
				}
				if g.statsSlide < 0.4 && inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
					mx, my := ebiten.CursorPosition()
					const tabW = 12
					if mx >= 0 && mx < tabW && my >= 50 && my < 50+54 {
						g.statsPanelOpen = !g.statsPanelOpen
					}
				}
				dx, dy := arrowDirection()
				if dx != g.lastSentMoveDx || dy != g.lastSentMoveDy {
					if err := gamews.Send(g.wsGame, gamekit.TypeMove, gamekit.MoveIntent{DX: dx, DY: dy}); err != nil {
						log.Printf("ws write: %v", err)
						return err
					}
					g.lastSentMoveDx, g.lastSentMoveDy = dx, dy
				}
				target, damage := hitPlayer()
				if damage != 0 && target != 0 {
					if err := gamews.Send(g.wsGame, gamekit.TypeHit, gamekit.HitIntent{TargetID: target, Damage: damage}); err != nil {
						log.Printf("ws write: %v", err)
						return err
					}
				}
				ww, wh := ebiten.WindowSize()
				if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
					if !g.consumeInventoryBarRMB(ww, wh) {
						g.tryInteractAtCursor()
					}
				}
				if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonMiddle) {
					g.tryPickupAtCursor()
				}
			}
			g.tickPlayerVis()
			g.tickDemoWalkAnims()
			g.pruneChatBubbles()
			g.tickCamera()
			ww, wh := ebiten.WindowSize()
			g.handleInventoryBarInput(ww, wh)
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

func hitPlayer() (target int64, damage int) {
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		damage = 1
		target = 4
	}
	return target, damage
}

// tryInteractAtCursor — ПКМ по клетке: верхний тайл с texture из каталога (interact) → WS interact.
func (g *Game) tryInteractAtCursor() {
	if len(g.interactTextures) == 0 {
		return
	}
	mx, my := ebiten.CursorPosition()
	cx, cy := g.viewCam()
	tx, ty, cell := world.TileFromScreenWithCam(mx, my, cx, cy)
	if !cell {
		return
	}
	var best *gamekit.Tile
	bestL := -100000
	for i := range g.World.Tiles {
		t := &g.World.Tiles[i]
		if t.X != tx || t.Y != ty {
			continue
		}
		if _, isInteract := g.interactTextures[t.Texture]; !isInteract {
			continue
		}
		if t.Layer > bestL {
			bestL = t.Layer
			best = t
		}
	}
	if best == nil {
		return
	}
	id := strings.TrimSpace(best.Texture)
	if id == "" {
		return
	}
	ix, iy := tx, ty
	L := best.Layer
	intent := gamekit.InteractIntent{
		ItemDefID:  id,
		ClickX:     &ix,
		ClickY:     &iy,
		ClickLayer: &L,
	}
	if err := gamews.Send(g.wsGame, gamekit.TypeInteract, intent); err != nil {
		log.Printf("ws interact: %v", err)
	}
}

func playerChebyshevAdjacentToCell(plX, plY, tx, ty int) bool {
	dx := plX - tx
	if dx < 0 {
		dx = -dx
	}
	dy := plY - ty
	if dy < 0 {
		dy = -dy
	}
	m := dx
	if dy > m {
		m = dy
	}
	return m <= 1
}

// tryPickupAtCursor — СКМ: pickable-тайл в соседней клетке (Чебышёв ≤ 1) → pickup_item.
func (g *Game) tryPickupAtCursor() {
	if len(g.pickableTextures) == 0 {
		return
	}
	pl, ok := g.World.Players[g.userID]
	if !ok {
		return
	}
	mx, my := ebiten.CursorPosition()
	cx, cy := g.viewCam()
	tx, ty, cell := world.TileFromScreenWithCam(mx, my, cx, cy)
	if !cell {
		return
	}
	if !playerChebyshevAdjacentToCell(pl.X, pl.Y, tx, ty) {
		return
	}
	var best *gamekit.Tile
	bestL := -100000
	for i := range g.World.Tiles {
		t := &g.World.Tiles[i]
		if t.X != tx || t.Y != ty {
			continue
		}
		if _, pick := g.pickableTextures[t.Texture]; !pick {
			continue
		}
		if t.Layer > bestL {
			bestL = t.Layer
			best = t
		}
	}
	if best == nil {
		return
	}
	id := strings.TrimSpace(best.Texture)
	if id == "" {
		return
	}
	ix, iy := tx, ty
	L := best.Layer
	intent := gamekit.PickupIntent{
		ItemDefID:  id,
		ClickX:     &ix,
		ClickY:     &iy,
		ClickLayer: &L,
	}
	if err := gamews.Send(g.wsGame, gamekit.TypePickupItem, intent); err != nil {
		log.Printf("ws pickup_item: %v", err)
	}
}

// cameraTarget мгновенная позиция камеры: центр окна на игроке (по сглаженным координатам); без игрока — (0,0).
func (g *Game) cameraTarget() (tx, ty float32) {
	pl, ok := g.World.Players[g.userID]
	if !ok {
		return 0, 0
	}
	vx, vy := float32(pl.X), float32(pl.Y)
	if v, ok := g.visTile[g.userID]; ok {
		vx, vy = v.X, v.Y
	}
	px, py := world.ToScreenCenterF(vx, vy)
	ww, wh := ebiten.WindowSize()
	return px - float32(ww)*0.5, py - float32(wh)*0.5
}

func (g *Game) tickStatsPanelSlide() {
	target := float32(0)
	if g.statsPanelOpen {
		target = 1
	}
	dt := float32(1.0 / 60.0)
	if tps := ebiten.ActualTPS(); tps > 1 {
		dt = float32(1.0 / tps)
	}
	f := float32(1 - math.Exp(-statsSlideLambda*float64(dt)))
	g.statsSlide += (target - g.statsSlide) * f
}

func (g *Game) tickCamera() {
	tx, ty := g.cameraTarget()
	dt := 1.0 / 60.0
	if tps := ebiten.ActualTPS(); tps > 1 {
		dt = 1.0 / tps
	}
	f := float32(1 - math.Exp(-camFollowLambda*dt))
	g.camX += (tx - g.camX) * f
	g.camY += (ty - g.camY) * f
}

func (g *Game) viewCam() (camX, camY float32) { return g.camX, g.camY }

func (g *Game) pruneChatBubbles() {
	now := time.Now()
	for id, b := range g.chatBubbles {
		if now.After(b.until) {
			delete(g.chatBubbles, id)
		}
	}
}

func (g *Game) tickDemoWalkAnims() {
	for id := range g.World.Players {
		g.tickDemoWalkFor(id)
	}
}

func (g *Game) tickDemoWalkFor(playerID int64) {
	pl, ok := g.World.Players[playerID]
	if !ok {
		g.demoWalkMoving[playerID] = false
		g.demoWalkPhase[playerID] = 0
		return
	}
	dt := 1.0 / 60.0
	if tps := ebiten.ActualTPS(); tps > 1 {
		dt = 1.0 / tps
	}
	vx, vy := float32(pl.X), float32(pl.Y)
	if v, ok2 := g.visTile[playerID]; ok2 {
		vx, vy = v.X, v.Y
	}
	dxf := float64(vx) - float64(pl.X)
	dyf := float64(vy) - float64(pl.Y)
	const eps = 0.018
	moving := dxf*dxf+dyf*dyf > eps*eps
	g.demoWalkMoving[playerID] = moving
	if moving {
		g.demoWalkPhase[playerID] += dt * 7
	} else {
		g.demoWalkPhase[playerID] = 0
	}
}

func (g *Game) tickPlayerVis() {
	dt := 1.0 / 60.0
	if tps := ebiten.ActualTPS(); tps > 1 {
		dt = 1.0 / tps
	}
	f := float32(1 - math.Exp(-playerVisLambda*dt))

	for id := range g.visTile {
		if _, ok := g.World.Players[id]; !ok {
			delete(g.visTile, id)
		}
	}
	for id, pl := range g.World.Players {
		tx, ty := float32(pl.X), float32(pl.Y)
		v, ok := g.visTile[id]
		if !ok {
			g.visTile[id] = struct{ X, Y float32 }{tx, ty}
			continue
		}
		dx, dy := tx-v.X, ty-v.Y
		if dx*dx+dy*dy > playerVisSnapTilesSq {
			v.X, v.Y = tx, ty
		} else {
			v.X += dx * f
			v.Y += dy * f
		}
		g.visTile[id] = v
	}
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{R: 0x18, G: 0x1c, B: 0x28, A: 0xff})

	camX, camY := g.viewCam()
	camOpts := tiles.DrawOpts{
		CamX: camX,
		CamY: camY,
		ResolveTexture: func(tex string) string {
			return tiles.ResolvedItemTexture(tex, func(id string) bool {
				return gamecontent.IsCatalogItemID(data.ContentCatalogJSON, id)
			})
		},
	}

	tileList := slices.Clone(g.World.Tiles)
	slices.SortFunc(tileList, func(a, b gamekit.Tile) int {
		if a.Y != b.Y {
			return a.Y - b.Y
		}
		if a.X != b.X {
			return a.X - b.X
		}
		return a.Layer - b.Layer
	})
	// Текстуры из state.Texture: Base_N из assets/tileSets и одиночные ключи = имя PNG в корне assets/.
	for _, t := range tileList {
		if t.Layer < playerTileLayer {
			tiles.Draw(screen, t, camOpts)
		}
	}

	ids := make([]int64, 0, len(g.World.Players))
	for id := range g.World.Players {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	face := &textv2.GoTextFace{
		Source: ui.FontSource(),
		Size:   world.LabelTextSize,
	}
	bubbleFace := &textv2.GoTextFace{Source: ui.FontSource(), Size: 12}

	for _, id := range ids {
		pl := g.World.Players[id]
		vx, vy := float32(pl.X), float32(pl.Y)
		if v, ok := g.visTile[id]; ok {
			vx, vy = v.X, v.Y
		}
		cx, cy := world.ToScreenCenterF(vx, vy)
		cx -= camX
		cy -= camY
		fill := playerColor(id)

		sprite := strings.TrimSpace(pl.Sprite)
		if sprite == "" {
			sprite = gamekit.DefaultPlayerSprite
		}
		if playeranim.WalkSheet(sprite) != nil {
			scale := float64(world.TileSize) / float64(playeranim.WalkFramePx)
			playeranim.DrawWalkSheet(screen, sprite, cx, cy, playeranim.CardinalFromPlayer(pl), g.demoWalkMoving[id], g.demoWalkPhase[id], scale)
		} else {
			vector.DrawFilledCircle(screen, cx, cy, world.PlayerRadius, fill, true)
			vector.StrokeCircle(screen, cx, cy, world.PlayerRadius, 1.5, color.RGBA{0xff, 0xff, 0xff, 0x90}, true)
			if pl.FaceDX != 0 || pl.FaceDY != 0 {
				const mark = float32(22)
				tipX := cx + float32(pl.FaceDX)*mark
				tipY := cy + float32(pl.FaceDY)*mark
				card := playeranim.CardinalFromPlayer(pl)
				arrow := [...]color.RGBA{
					{0xff, 0xc8, 0x78, 0xee}, // E
					{0x88, 0xd8, 0xff, 0xee}, // S
					{0xc8, 0xa8, 0xff, 0xee}, // W
					{0x98, 0xf0, 0xa8, 0xee}, // N
				}
				vector.StrokeLine(screen, cx, cy, tipX, tipY, 2.5, arrow[card], true)
			}
		}

		tagY := float64(cy) - float64(world.PlayerRadius) - world.LabelAboveGap
		idLabel := fmt.Sprintf("%d", id)
		_, idH := textv2.Measure(idLabel, face, 0)
		if b, ok := g.chatBubbles[id]; ok && b.text != "" {
			const bubblePad = 6.0
			const bubbleGap = 6.0 // зазор между низом пузыря и верхом строки ID
			tw, th := textv2.Measure(b.text, bubbleFace, 0)
			bw := float32(tw) + float32(2*bubblePad)
			bh := float32(th) + float32(2*bubblePad)
			// tagY — низ подписи ID; пузырь целиком выше блока ID
			bubbleBottom := float32(tagY) - float32(idH+bubbleGap)
			bubbleTop := bubbleBottom - bh
			bx := cx - bw*0.5
			vector.DrawFilledRect(screen, bx, bubbleTop, bw, bh, color.RGBA{0x22, 0x24, 0x32, 0xea}, false)
			vector.StrokeRect(screen, bx, bubbleTop, bw, bh, 1.5, color.RGBA{0x6e, 0x82, 0xa8, 0xff}, false)
			bo := &textv2.DrawOptions{}
			bo.PrimaryAlign = textv2.AlignCenter
			bo.SecondaryAlign = textv2.AlignCenter
			bo.GeoM.Translate(float64(cx), float64(bubbleTop)+float64(bh)*0.5)
			bo.ColorScale.ScaleWithColor(color.RGBA{0xee, 0xf0, 0xf8, 0xff})
			textv2.Draw(screen, b.text, bubbleFace, bo)
		}

		opts := &textv2.DrawOptions{}
		opts.PrimaryAlign = textv2.AlignCenter
		opts.SecondaryAlign = textv2.AlignEnd
		opts.GeoM.Translate(float64(cx), tagY)
		opts.ColorScale.ScaleWithColor(color.RGBA{0xff, 0xff, 0xff, 0xff})
		textv2.Draw(screen, idLabel, face, opts)
	}

	for _, t := range tileList {
		if t.Layer >= playerTileLayer {
			tiles.Draw(screen, t, camOpts)
		}

	}

	if g.worldFog != nil {
		g.worldFog.Draw(screen, g.World.Tiles, camX, camY, g.fogMode)
	}

	g.drawStatsPanel(screen)
	sw, sh := screen.Bounds().Dx(), screen.Bounds().Dy()
	g.drawInventoryBar(screen, sw, sh)
	g.drawLobbyChat(screen)
	g.drawPerfHud(screen)
}

func abMod(v int) int { return (v - 10) / 2 }

func fogModeTag(m mapfog.RenderMode) string {
	switch m {
	case mapfog.RenderBlurAndFog:
		return "туман+блюр"
	case mapfog.RenderFogOnly:
		return "туман"
	case mapfog.RenderBlurOnly:
		return "блюр"
	case mapfog.RenderNone:
		return "выкл"
	default:
		return "?"
	}
}

func (g *Game) drawStatsPanel(screen *ebiten.Image) {
	px := float32(14) - (1-g.statsSlide)*float32(statsPanelW+28)
	py := float32(14)

	title := g.characterDisplayName
	if title == "" {
		title = fmt.Sprintf("Игрок %d", g.userID)
	}

	pl, ok := g.World.Players[g.userID]
	extraSkin := float32(0)
	if ok {
		extraSkin = statsLineSize + 4
	}
	bodyH := float32(8 + statsTitleSize + 6 + statsLineSize + 4 + extraSkin + 6*statsLineSize + 2*statsHintSize + 22)
	vector.DrawFilledRect(screen, px, py, statsPanelW, bodyH, color.RGBA{0x14, 0x16, 0x22, 0xee}, false)
	vector.StrokeRect(screen, px, py, statsPanelW, bodyH, 1, color.RGBA{0x5a, 0x68, 0x88, 0xff}, false)

	titleFace := &textv2.GoTextFace{Source: ui.FontSource(), Size: statsTitleSize}
	lineFace := &textv2.GoTextFace{Source: ui.FontSource(), Size: statsLineSize}
	hintFace := &textv2.GoTextFace{Source: ui.FontSource(), Size: statsHintSize}

	ty := py + 8.0
	to := &textv2.DrawOptions{}
	to.GeoM.Translate(float64(px+10), float64(ty))
	to.ColorScale.ScaleWithColor(color.RGBA{0xf2, 0xf4, 0xfc, 0xff})
	textv2.Draw(screen, title, titleFace, to)
	ty += float32(statsTitleSize + 6)

	if !ok {
		lo := &textv2.DrawOptions{}
		lo.GeoM.Translate(float64(px+10), float64(ty))
		lo.ColorScale.ScaleWithColor(color.RGBA{0xa8, 0xb0, 0xc8, 0xff})
		textv2.Draw(screen, "Ожидание состояния…", lineFace, lo)
		ty += statsLineSize + 8
	} else {
		hpLine := fmt.Sprintf("HP  %d", pl.HP)
		ho := &textv2.DrawOptions{}
		ho.GeoM.Translate(float64(px+10), float64(ty))
		ho.ColorScale.ScaleWithColor(color.RGBA{0xc8, 0xe8, 0xd0, 0xff})
		textv2.Draw(screen, hpLine, lineFace, ho)
		ty += statsLineSize + 4
		sk := strings.TrimSpace(pl.Sprite)
		if sk == "" {
			sk = gamekit.DefaultPlayerSprite
		}
		sko := &textv2.DrawOptions{}
		sko.GeoM.Translate(float64(px+10), float64(ty))
		sko.ColorScale.ScaleWithColor(color.RGBA{0xb8, 0xc8, 0xe8, 0xff})
		textv2.Draw(screen, fmt.Sprintf("Скин  %s", sk), lineFace, sko)
		ty += statsLineSize + 6

		s := pl.Stats
		rows := []struct {
			label string
			val   int
		}{
			{"Сила", s.Strength},
			{"Ловкость", s.Dexterity},
			{"Телосложение", s.Constitution},
			{"Интеллект", s.Intelligence},
			{"Мудрость", s.Wisdom},
			{"Харизма", s.Charisma},
		}
		for _, row := range rows {
			m := abMod(row.val)
			line := fmt.Sprintf("%-14s  %2d  (%+d)", row.label, row.val, m)
			ro := &textv2.DrawOptions{}
			ro.GeoM.Translate(float64(px+10), float64(ty))
			ro.ColorScale.ScaleWithColor(color.RGBA{0xd8, 0xdc, 0xec, 0xff})
			textv2.Draw(screen, line, lineFace, ro)
			ty += statsLineSize + 2
		}
		ty += 4
	}

	hin := &textv2.DrawOptions{}
	hin.GeoM.Translate(float64(px+10), float64(ty))
	hin.ColorScale.ScaleWithColor(color.RGBA{0x88, 0x90, 0xa8, 0xff})
	textv2.Draw(screen, "ПКМ — взаимодействие (interact), СКМ — подобрать (pickable)", hintFace, hin)
	ty += statsHintSize + 4
	hin.GeoM.Reset()
	hin.GeoM.Translate(float64(px+10), float64(ty))
	hin.ColorScale.ScaleWithColor(color.RGBA{0x88, 0x90, 0xa8, 0xff})
	textv2.Draw(screen, "C — скрыть / показать панель", hintFace, hin)

	// полоска-«ручка», когда панель почти уехала
	if g.statsSlide < 0.35 {
		tabW := float32(10.0)
		tabH := float32(52.0)
		tabX := float32(0)
		tabY := py + 36.0
		vector.DrawFilledRect(screen, tabX, tabY, tabW, tabH, color.RGBA{0x28, 0x2c, 0x3c, 0xf0}, false)
		vector.StrokeRect(screen, tabX, tabY, tabW, tabH, 1, color.RGBA{0x6a, 0x78, 0x98, 0xff}, false)
		glyph := "›"
		if !g.statsPanelOpen {
			glyph = "‹"
		}
		gf := &textv2.GoTextFace{Source: ui.FontSource(), Size: 16}
		go2 := &textv2.DrawOptions{}
		go2.PrimaryAlign = textv2.AlignCenter
		go2.SecondaryAlign = textv2.AlignCenter
		go2.GeoM.Translate(float64(tabX+tabW*0.5), float64(tabY+tabH*0.5))
		go2.ColorScale.ScaleWithColor(color.RGBA{0xc8, 0xd0, 0xe8, 0xff})
		textv2.Draw(screen, glyph, gf, go2)
	}
}

func playerColor(id int64) color.RGBA {
	ii := int(id)
	h := uint8(37*ii + 80)
	return color.RGBA{R: h, G: uint8(200 - (ii*17)%80), B: uint8(120 + (ii*13)%100), A: 0xff}
}

func (g *Game) sendLobbyLine() {
	text := strings.TrimSpace(g.lobbyDraft)
	if text == "" {
		return
	}
	g.lobbyDraft = ""
	if err := lobby.SendLine(g.jwt, g.lobbyChatID, g.userID, text); err != nil {
		log.Printf("lobby send: %v", err)
	}
}

func (g *Game) drawPerfHud(screen *ebiten.Image) {
	b := screen.Bounds()
	ww, wh := b.Dx(), b.Dy()
	if ww <= 0 || wh <= 0 {
		return
	}
	line := fmt.Sprintf("FPS %.1f  TPS %.1f  [%s] F4", ebiten.ActualFPS(), ebiten.ActualTPS(), fogModeTag(g.fogMode))
	face := &textv2.GoTextFace{Source: ui.FontSource(), Size: 12}
	_, th := textv2.Measure(line, face, 0)
	const margin = 10.0
	x := margin
	y := float32(wh) - float32(th) - margin

	sh := &textv2.DrawOptions{}
	sh.GeoM.Translate(float64(x+1), float64(y+1))
	sh.ColorScale.ScaleWithColor(color.RGBA{0x10, 0x12, 0x18, 0xb0})
	textv2.Draw(screen, line, face, sh)
	op := &textv2.DrawOptions{}
	op.GeoM.Translate(float64(x), float64(y))
	op.ColorScale.ScaleWithColor(color.RGBA{0xe8, 0xec, 0xf8, 0xff})
	textv2.Draw(screen, line, face, op)
}

func (g *Game) drawLobbyChat(screen *ebiten.Image) {
	w, _ := ebiten.WindowSize()
	const panelW float32 = 400
	panelH := float32(WindowHeight) * 0.42
	x := float32(w) - panelW - 14
	y := float32(14)
	vector.DrawFilledRect(screen, x, y, panelW, panelH, color.RGBA{0x18, 0x18, 0x22, 0xd8}, false)
	vector.StrokeRect(screen, x, y, panelW, panelH, 1, color.RGBA{0x66, 0x66, 0x77, 0xff}, false)

	titleFace := &textv2.GoTextFace{Source: ui.FontSource(), Size: 14}
	titleOpts := &textv2.DrawOptions{}
	titleOpts.GeoM.Translate(float64(x+10), float64(y+6))
	titleOpts.ColorScale.ScaleWithColor(color.RGBA{0xff, 0xff, 0xff, 0xff})
	textv2.Draw(screen, "Лобби", titleFace, titleOpts)

	face := &textv2.GoTextFace{Source: ui.FontSource(), Size: g.lobbyChatSize}
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
		s := fmt.Sprintf("[%d] %s", ln.SenderID, ln.Text)
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
