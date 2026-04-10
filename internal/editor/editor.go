package editor

import (
	"encoding/json"
	"fmt"
	"image/color"
	"log"
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

	"client/internal/gamews"
	"client/internal/state"
	"client/internal/tiles"
	"client/internal/ui"
	"client/internal/world"
)

const (
	WindowWidth  = 1600
	WindowHeight = 800

	maxEditLayer     = 31
	maxSaveNameRunes = 120
	saveToastDur     = 7 * time.Second
	cameraPanSpeed   = float32(14) // px за кадр при удержании стрелки
)

// App — клиент редактора мира: spawn_tile по клику, превью состояния с game WS.
type App struct {
	wsGame *websocket.Conn
	msgs   <-chan gamekit.Envelope
	World  *state.World

	setNames []string
	setIdx   int
	tileIdx  int // 0-based внутри набора → на wire Beach_Tile_{tileIdx+1}
	blocks   bool

	pickTilesets   bool // true: тайлсеты из assets/tileSets, false: корневые PNG в assets/
	singleIdx      int
	paletteScroll  int // вертикаль сетки палитры
	paletteScrollX int // горизонталь (если сетка шире панели)
	winW, winH     int

	camX, camY float32 // камера: мир сдвигается на экране (px)

	editLayer    int // слой spawn_tile / clear_tile
	editRotation int // четверти по часовой, 0..3

	savePanelOpen     bool
	saveNameDraft     string
	saveToast         string
	saveToastBad      bool // true — ошибка (другой цвет)
	saveToastDeadline time.Time

	paintNX, paintNY int
	paintHave        bool
	rectDrag         bool
	rectX0, rectY0   int
	rectX1, rectY1   int
}

func New(wsGame *websocket.Conn, msgs <-chan gamekit.Envelope) *App {
	_ = ui.FontSource()
	sets := tiles.EditorTileSets()
	si := 0
	for i, s := range sets {
		if tiles.TileCountInSet(s) > 0 {
			si = i
			break
		}
	}
	return &App{
		wsGame:       wsGame,
		msgs:         msgs,
		World:        state.NewWorld(),
		setNames:     sets,
		setIdx:       si,
		tileIdx:      0,
		blocks:       true,
		pickTilesets: true,
		singleIdx:    0,
		winW:         WindowWidth,
		winH:         WindowHeight,
		editLayer:    0,
		editRotation: 0,
	}
}

func (a *App) incLayer() {
	if a.editLayer < maxEditLayer {
		a.editLayer++
	}
}

func (a *App) decLayer() {
	if a.editLayer > 0 {
		a.editLayer--
	}
}

func (a *App) stepRotation(delta int) {
	a.editRotation = tiles.NormalizeRotationQuarter(a.editRotation + delta)
}

func (a *App) shiftPaletteKeys() bool {
	return ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)
}

func (a *App) ctrlHeld() bool {
	return ebiten.IsKeyPressed(ebiten.KeyControlLeft) || ebiten.IsKeyPressed(ebiten.KeyControlRight)
}

func (a *App) sendSpawn(tx, ty int) {
	intent := gamekit.TileSpawnIntent{
		X:        tx,
		Y:        ty,
		Layer:    a.editLayer,
		Rotation: tiles.NormalizeRotationQuarter(a.editRotation),
		Texture:  a.texture(),
		Blocks:   a.blocks,
	}
	if err := gamews.Send(a.wsGame, gamekit.TypeSpawnTile, intent); err != nil {
		log.Printf("editor spawn_tile: %v", err)
	}
}

func (a *App) fillSpawnRect(x0, y0, x1, y1 int) {
	xa, xb := min(x0, x1), max(x0, x1)
	ya, yb := min(y0, y1), max(y0, y1)
	for y := ya; y <= yb; y++ {
		for x := xa; x <= xb; x++ {
			a.sendSpawn(x, y)
		}
	}
}

func (a *App) drawRectSel(screen *ebiten.Image) {
	xa, xb := min(a.rectX0, a.rectX1), max(a.rectX0, a.rectX1)
	ya, yb := min(a.rectY0, a.rectY1), max(a.rectY0, a.rectY1)
	ts := float32(world.TileSize)
	gp := float32(world.GridPad)
	fx := gp + float32(xa)*ts - a.camX
	fy := gp + float32(ya)*ts - a.camY
	fw := float32(xb-xa+1) * ts
	fh := float32(yb-ya+1) * ts
	vector.DrawFilledRect(screen, fx, fy, fw, fh, color.RGBA{0x50, 0xa8, 0xf8, 0x22}, false)
	vector.StrokeRect(screen, fx, fy, fw, fh, 2, color.RGBA{0x88, 0xd0, 0xff, 0xcc}, false)
}

func (a *App) currentSet() string {
	if len(a.setNames) == 0 {
		return ""
	}
	a.setIdx = (a.setIdx + len(a.setNames)) % len(a.setNames)
	return a.setNames[a.setIdx]
}

func (a *App) tileCount() int {
	return tiles.TileCountInSet(a.currentSet())
}

func (a *App) clampTileIdx() {
	n := a.tileCount()
	if n <= 0 {
		a.tileIdx = 0
		return
	}
	if a.tileIdx >= n {
		a.tileIdx = n - 1
	}
	if a.tileIdx < 0 {
		a.tileIdx = 0
	}
}

func (a *App) clampSingleIdx() {
	keys := tiles.EditorSingleTextureKeys()
	if len(keys) == 0 {
		a.singleIdx = 0
		return
	}
	if a.singleIdx < 0 {
		a.singleIdx = 0
	}
	if a.singleIdx >= len(keys) {
		a.singleIdx = len(keys) - 1
	}
}

func (a *App) texture() string {
	if !a.pickTilesets {
		keys := tiles.EditorSingleTextureKeys()
		if len(keys) == 0 {
			return "wall"
		}
		a.clampSingleIdx()
		return keys[a.singleIdx]
	}
	n := a.tileCount()
	if n == 0 {
		return "wall"
	}
	a.clampTileIdx()
	return tiles.TextureKey(a.currentSet(), a.tileIdx)
}

func (a *App) drainWebSocket() error {
	for {
		select {
		case msg, ok := <-a.msgs:
			if !ok {
				return fmt.Errorf("websocket closed")
			}
			if msg.Service == gamekit.ServiceGame && msg.Type == gamekit.TypeSaveWorldResult {
				var p gamekit.SaveWorldResultPayload
				if err := json.Unmarshal(msg.Payload, &p); err != nil {
					log.Printf("editor save_world_result: %v", err)
					continue
				}
				if p.Ok {
					a.saveToastBad = false
					a.saveToast = fmt.Sprintf("Сохранено: «%s» · v%d · %s", p.Name, p.Version, p.WorldID)
				} else {
					a.saveToastBad = true
					a.saveToast = fmt.Sprintf("%s — %s", p.Code, p.Message)
				}
				a.saveToastDeadline = time.Now().Add(saveToastDur)
				a.savePanelOpen = false
				continue
			}
			a.World.ApplyEnvelope(msg)
		default:
			return nil
		}
	}
}

func (a *App) trySaveWorld() {
	name := strings.TrimSpace(a.saveNameDraft)
	if name == "" {
		a.saveToastBad = true
		a.saveToast = "Укажите имя мира (не пустое)"
		a.saveToastDeadline = time.Now().Add(4 * time.Second)
		return
	}
	if err := gamews.Send(a.wsGame, gamekit.TypeSaveWorld, gamekit.SaveWorldIntent{Name: name}); err != nil {
		log.Printf("editor save_world: %v", err)
		a.saveToastBad = true
		a.saveToast = fmt.Sprintf("Ошибка отправки: %v", err)
		a.saveToastDeadline = time.Now().Add(saveToastDur)
	}
}

func (a *App) nextSet(delta int) {
	if len(a.setNames) == 0 {
		return
	}
	for k := 0; k < len(a.setNames); k++ {
		a.setIdx = (a.setIdx + delta + len(a.setNames)) % len(a.setNames)
		if tiles.TileCountInSet(a.currentSet()) > 0 {
			a.clampTileIdx()
			return
		}
	}
}

func (a *App) Update() error {
	if err := a.drainWebSocket(); err != nil {
		return err
	}
	if a.saveToast != "" && !a.saveToastDeadline.IsZero() && time.Now().After(a.saveToastDeadline) {
		a.saveToast = ""
	}

	if a.savePanelOpen {
		if inpututil.IsKeyJustPressed(ebiten.KeyF2) {
			a.savePanelOpen = false
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
			a.savePanelOpen = false
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
			a.trySaveWorld()
		}
		if ebiten.IsFocused() {
			for _, ch := range ebiten.AppendInputChars(nil) {
				if ch == '\n' || ch == '\r' {
					continue
				}
				if utf8.RuneCountInString(a.saveNameDraft) >= maxSaveNameRunes {
					break
				}
				a.saveNameDraft += string(ch)
			}
			if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && a.saveNameDraft != "" {
				_, sz := utf8.DecodeLastRuneInString(a.saveNameDraft)
				a.saveNameDraft = a.saveNameDraft[:len(a.saveNameDraft)-sz]
			}
		}
		return nil
	}

	mx, my := ebiten.CursorPosition()
	wx, wy := ebiten.Wheel()
	a.handlePaletteScroll(mx, my, wx, wy)

	if inpututil.IsKeyJustPressed(ebiten.KeyF2) {
		a.savePanelOpen = true
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyComma) {
		a.decLayer()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyPeriod) {
		a.incLayer()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyR) {
		a.stepRotation(1)
	}

	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		a.blocks = !a.blocks
	}
	if !a.shiftPaletteKeys() {
		if ebiten.IsKeyPressed(ebiten.KeyArrowLeft) {
			a.camX -= cameraPanSpeed
		}
		if ebiten.IsKeyPressed(ebiten.KeyArrowRight) {
			a.camX += cameraPanSpeed
		}
		if ebiten.IsKeyPressed(ebiten.KeyArrowUp) {
			a.camY -= cameraPanSpeed
		}
		if ebiten.IsKeyPressed(ebiten.KeyArrowDown) {
			a.camY += cameraPanSpeed
		}
	} else if a.pickTilesets {
		if inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft) {
			n := a.tileCount()
			if n > 0 {
				a.tileIdx = (a.tileIdx - 1 + n) % n
			}
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyArrowRight) {
			n := a.tileCount()
			if n > 0 {
				a.tileIdx = (a.tileIdx + 1) % n
			}
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) {
			a.nextSet(-1)
			a.paletteScroll, a.paletteScrollX = 0, 0
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
			a.nextSet(1)
			a.paletteScroll, a.paletteScrollX = 0, 0
		}
	} else {
		keys := tiles.EditorSingleTextureKeys()
		n := len(keys)
		if n > 0 {
			if inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft) || inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) {
				a.singleIdx = (a.singleIdx - 1 + n) % n
			}
			if inpututil.IsKeyJustPressed(ebiten.KeyArrowRight) || inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
				a.singleIdx = (a.singleIdx + 1) % n
			}
		}
	}
	ctrl := a.ctrlHeld()
	inPal := a.inPalette(mx, my)

	if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		if a.rectDrag {
			a.fillSpawnRect(a.rectX0, a.rectY0, a.rectX1, a.rectY1)
			a.rectDrag = false
		}
		a.paintHave = false
	}

	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		if !a.handlePaletteClick(mx, my) {
			if tx, ty, ok := a.mapTileFromCursor(mx, my); ok {
				if ctrl {
					a.rectDrag = true
					a.rectX0, a.rectY0 = tx, ty
					a.rectX1, a.rectY1 = tx, ty
				} else {
					a.sendSpawn(tx, ty)
					a.paintHave = true
					a.paintNX, a.paintNY = tx, ty
				}
			}
		}
	}
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) && !inPal {
		if tx, ty, ok := a.mapTileFromCursor(mx, my); ok {
			if ctrl {
				if a.rectDrag {
					a.rectX1, a.rectY1 = tx, ty
				}
			} else if a.paintHave {
				if tx != a.paintNX || ty != a.paintNY {
					a.sendSpawn(tx, ty)
					a.paintNX, a.paintNY = tx, ty
				}
			}
		}
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
		if !a.inPalette(mx, my) {
			if tx, ty, ok := a.mapTileFromCursor(mx, my); ok {
				cl := gamekit.TileClearIntent{X: tx, Y: ty, Layer: a.editLayer}
				if err := gamews.Send(a.wsGame, gamekit.TypeClearTile, cl); err != nil {
					log.Printf("editor clear_tile: %v", err)
				}
			}
		}
	}
	return nil
}

func (a *App) Draw(screen *ebiten.Image) {
	screen.Clear()

	tileList := slices.Clone(a.World.Tiles)
	slices.SortFunc(tileList, func(x, y gamekit.Tile) int {
		if x.Y != y.Y {
			return x.Y - y.Y
		}
		if x.X != y.X {
			return x.X - y.X
		}
		return x.Layer - y.Layer
	})
	camOpts := tiles.DrawOpts{OutlineBlocking: true, CamX: a.camX, CamY: a.camY}
	for _, t := range tileList {
		tiles.Draw(screen, t, camOpts)
	}

	if !a.savePanelOpen {
		cmx, cmy := ebiten.CursorPosition()
		if !a.inPalette(cmx, cmy) {
			if a.rectDrag {
				a.drawRectSel(screen)
			}
			if tx, ty, ok := a.mapTileFromCursor(cmx, cmy); ok {
				ga := float32(0.45)
				if a.rectDrag {
					ga = 0.38
				}
				tiles.DrawGhost(screen, tx, ty, a.texture(), a.editRotation, a.blocks, camOpts, ga)
			}
		}
	}

	ids := make([]int64, 0, len(a.World.Players))
	for id := range a.World.Players {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	face := &textv2.GoTextFace{Source: ui.FontSource(), Size: world.LabelTextSize}
	for _, id := range ids {
		pl := a.World.Players[id]
		cx, cy := world.ToScreen(pl.X, pl.Y)
		cx -= a.camX
		cy -= a.camY
		fill := playerColor(id)
		vector.DrawFilledCircle(screen, cx, cy, world.PlayerRadius, fill, true)
		vector.StrokeCircle(screen, cx, cy, world.PlayerRadius, 1.5, color.RGBA{0xff, 0xff, 0xff, 0x90}, true)
		label := fmt.Sprintf("%d", pl.HP)
		opts := &textv2.DrawOptions{}
		opts.PrimaryAlign = textv2.AlignCenter
		opts.SecondaryAlign = textv2.AlignEnd
		opts.GeoM.Translate(float64(cx), float64(cy)-world.PlayerRadius-world.LabelAboveGap)
		opts.ColorScale.ScaleWithColor(color.RGBA{0xff, 0xff, 0xff, 0xff})
		textv2.Draw(screen, label, face, opts)
	}

	a.drawPalette(screen)

	line1 := "Стрелки — камера · Shift+стрелки — палитра · ЛКМ кисть · Ctrl+ЛКМ прямоугольник (отпустить) · ПКМ стереть · F2 сохранить · , . слой · R поворот"
	hudFace := &textv2.GoTextFace{Source: ui.FontSource(), Size: 14}
	hudOpts := &textv2.DrawOptions{}
	hudOpts.ColorScale.ScaleWithColor(color.RGBA{0xf0, 0xf0, 0xf0, 0xff})
	hudOpts.GeoM.Translate(12, 8)
	textv2.Draw(screen, line1, hudFace, hudOpts)

	if a.saveToast != "" {
		toastFace := &textv2.GoTextFace{Source: ui.FontSource(), Size: 13}
		toastOpts := &textv2.DrawOptions{}
		toastOpts.PrimaryAlign = textv2.AlignCenter
		toastOpts.GeoM.Translate(float64(a.winW)/2, 36)
		tc := color.RGBA{0xa8, 0xf0, 0xb8, 0xff}
		if a.saveToastBad {
			tc = color.RGBA{0xff, 0xd0, 0x88, 0xff}
		}
		toastOpts.ColorScale.ScaleWithColor(tc)
		s := a.saveToast
		if utf8.RuneCountInString(s) > 90 {
			s = string([]rune(s)[:87]) + "…"
		}
		textv2.Draw(screen, s, toastFace, toastOpts)
	}

	if a.savePanelOpen {
		a.drawSavePanel(screen)
	}
}

func (a *App) drawSavePanel(screen *ebiten.Image) {
	w, h := float32(a.winW), float32(a.winH)
	vector.DrawFilledRect(screen, 0, 0, w, h, color.RGBA{0x10, 0x10, 0x18, 0xbd}, false)

	const boxW, boxH float32 = 440, 130
	bx := (w - boxW) / 2
	by := (h - boxH) / 2
	vector.DrawFilledRect(screen, bx, by, boxW, boxH, color.RGBA{0x28, 0x28, 0x34, 0xff}, false)
	vector.StrokeRect(screen, bx, by, boxW, boxH, 1.5, color.RGBA{0x66, 0x66, 0x80, 0xff}, false)

	face := &textv2.GoTextFace{Source: ui.FontSource(), Size: 15}
	small := &textv2.GoTextFace{Source: ui.FontSource(), Size: 12}
	opts := &textv2.DrawOptions{}
	opts.ColorScale.ScaleWithColor(color.RGBA{0xee, 0xee, 0xf5, 0xff})
	opts.GeoM.Translate(float64(bx+14), float64(by+12))
	textv2.Draw(screen, "Сохранить мир в world-service", face, opts)

	opts.GeoM.Reset()
	opts.GeoM.Translate(float64(bx+14), float64(by+42))
	opts.ColorScale.ScaleWithColor(color.RGBA{0xc8, 0xc8, 0xd8, 0xff})
	preview := a.saveNameDraft
	if preview == "" {
		preview = "…имя…"
		opts.ColorScale.ScaleWithColor(color.RGBA{0x88, 0x88, 0x98, 0xff})
	}
	textv2.Draw(screen, preview, small, opts)

	opts.GeoM.Reset()
	opts.GeoM.Translate(float64(bx+14), float64(by+88))
	opts.ColorScale.ScaleWithColor(color.RGBA{0x90, 0x90, 0xa0, 0xff})
	textv2.Draw(screen, "Enter — отправить   Esc / F2 — закрыть   (нужен save_world_admin_user_id на сервере)", small, opts)
}

func playerColor(id int64) color.RGBA {
	ii := int(id)
	h := uint8(37*ii + 80)
	return color.RGBA{R: h, G: uint8(200 - (ii*17)%80), B: uint8(120 + (ii*13)%100), A: 0xff}
}

func (a *App) Layout(outsideWidth, outsideHeight int) (int, int) {
	a.winW = outsideWidth
	a.winH = outsideHeight
	a.clampScroll()
	return outsideWidth, outsideHeight
}
