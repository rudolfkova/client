package editor

import (
	"encoding/json"
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
	"client/internal/gamews"
	"client/internal/gamecontent"
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
	// catalogItemLayer — слой «предметов на земле» (как в игре: < playerTileLayer).
	catalogItemLayer = 1

	camZoomMin = float32(0.25)
	camZoomMax       = float32(4)
	camZoomWheelStep = float32(1.09) // множитель за единицу Wheel() по Y
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
	pickCatalog    bool // вкладка «Предметы»: spawn_tile texture=id из catalog, слой catalogItemLayer
	catalogInteractIDs []string
	catItemIdx           int
	singleIdx            int
	paletteScroll  int // вертикаль сетки палитры
	paletteScrollX int // горизонталь (если сетка шире панели)
	paletteWidth   int // ширина панели палитры (правая колонка); край — тянуть
	paletteDrag    bool
	paletteDragX   int
	winW, winH     int

	camX, camY   float32 // камера: мир в «логических» px до масштаба
	camZoom      float32 // только редактор: экран = (мир − cam) * camZoom

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

	eraseNX, eraseNY int
	eraseHave        bool
	rectEraseDrag    bool
	rectEX0, rectEY0 int
	rectEX1, rectEY1 int

	spawnArgsOpen            bool
	spawnArgsPX, spawnArgsPY int
	spawnArgsDraft           string
	spawnArgsErr             string
	spawnArgsCursor          int // байтовое смещение в spawnArgsDraft
	spawnArgsFirstLine       int // первая видимая строка (для длинного JSON)

	catalogInteractSet map[string]struct{} // texture == id предмета с interact (жёлтая обводка)
	showTileOverlay    bool                 // F3: координаты и имя предмета на тайлах
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
	catIDs := gamecontent.InteractItemIDs(data.ContentCatalogJSON)
	catSet := make(map[string]struct{}, len(catIDs))
	for _, id := range catIDs {
		catSet[id] = struct{}{}
	}
	return &App{
		wsGame:               wsGame,
		msgs:                 msgs,
		World:                state.NewWorld(),
		setNames:             sets,
		setIdx:               si,
		tileIdx:              0,
		blocks:               true,
		pickTilesets:         true,
		catalogInteractIDs:   catIDs,
		catalogInteractSet:   catSet,
		singleIdx:            0,
		paletteWidth: paletteMinW,
		winW:         WindowWidth,
		winH:         WindowHeight,
		editLayer:    0,
		editRotation: 0,
		camZoom:      1,
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

func (a *App) sendSpawnCatalog(tx, ty int, instanceArgs json.RawMessage) {
	id := a.texture()
	if id == "" || id == "wall" {
		return
	}
	intent := gamekit.TileSpawnIntent{
		X: tx, Y: ty, Layer: catalogItemLayer, Rotation: 0, Texture: id, Blocks: false,
	}
	if len(instanceArgs) > 0 {
		intent.InstanceArgs = instanceArgs
	}
	if err := gamews.Send(a.wsGame, gamekit.TypeSpawnTile, intent); err != nil {
		log.Printf("editor spawn_tile: %v", err)
	}
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

func (a *App) sendClear(tx, ty int) {
	layer := a.editLayer
	if a.pickCatalog {
		layer = catalogItemLayer
	}
	cl := gamekit.TileClearIntent{X: tx, Y: ty, Layer: layer}
	if err := gamews.Send(a.wsGame, gamekit.TypeClearTile, cl); err != nil {
		log.Printf("editor clear_tile: %v", err)
	}
}

func (a *App) fillClearRect(x0, y0, x1, y1 int) {
	xa, xb := min(x0, x1), max(x0, x1)
	ya, yb := min(y0, y1), max(y0, y1)
	for y := ya; y <= yb; y++ {
		for x := xa; x <= xb; x++ {
			a.sendClear(x, y)
		}
	}
}

func (a *App) drawTileRectSel(screen *ebiten.Image, x0, y0, x1, y1 int, fill, stroke color.RGBA) {
	xa, xb := min(x0, x1), max(x0, x1)
	ya, yb := min(y0, y1), max(y0, y1)
	z := a.camZoomEffective()
	ts := float32(world.TileSize)
	gp := float32(world.GridPad)
	fx := (gp + float32(xa)*ts - a.camX) * z
	fy := (gp + float32(ya)*ts - a.camY) * z
	fw := float32(xb-xa+1) * ts * z
	fh := float32(yb-ya+1) * ts * z
	vector.DrawFilledRect(screen, fx, fy, fw, fh, fill, false)
	st := float32(2) * z
	if st < 1 {
		st = 1
	}
	vector.StrokeRect(screen, fx, fy, fw, fh, st, stroke, false)
}

// drawEditorTileChrome — жёлтая внутренняя обводка у тайлов-предметов (каталог с interact); по F3 — подписи клетки и имени.
func (a *App) drawEditorTileChrome(screen *ebiten.Image, tileList []gamekit.Tile) {
	z := a.camZoomEffective()
	gp := float32(world.GridPad)
	ts0 := float32(world.TileSize)
	ts := ts0 * z

	sz := 11.0 * float64(z)
	if sz < 9 {
		sz = 9
	}
	face := &textv2.GoTextFace{Source: ui.FontSource(), Size: sz}

	for _, t := range tileList {
		x0 := (gp + float32(t.X)*ts0 - a.camX) * z
		y0 := (gp + float32(t.Y)*ts0 - a.camY) * z
		tex := strings.TrimSpace(t.Texture)

		if _, isItem := a.catalogInteractSet[tex]; isItem {
			inset := float32(3.5) * z
			if inset < 1.2 {
				inset = 1.2
			}
			maxIn := ts * float32(0.22)
			if inset > maxIn {
				inset = maxIn
			}
			inner := ts - 2*inset
			if inner > 2 {
				vector.StrokeRect(screen, x0+inset, y0+inset, inner, inner, 1.2,
					color.RGBA{0xee, 0xcc, 0x44, 0xe5}, false)
			}
		}

		if !a.showTileOverlay {
			continue
		}

		cx := x0 + ts*0.5
		cy := y0 + ts*0.5
		lab := fmt.Sprintf("%d,%d", t.X, t.Y)
		opts := &textv2.DrawOptions{}
		opts.PrimaryAlign = textv2.AlignCenter
		opts.SecondaryAlign = textv2.AlignCenter
		opts.GeoM.Translate(float64(cx+1), float64(cy-2*z+1))
		opts.ColorScale.ScaleWithColor(color.RGBA{0x12, 0x14, 0x1c, 0x9a})
		textv2.Draw(screen, lab, face, opts)
		opts.GeoM.Reset()
		opts.GeoM.Translate(float64(cx), float64(cy-2*z))
		opts.ColorScale.ScaleWithColor(color.RGBA{0xf6, 0xf7, 0xfb, 0xee})
		textv2.Draw(screen, lab, face, opts)

		if _, isItem := a.catalogInteractSet[tex]; isItem {
			name := gamecontent.ItemDisplayName(data.ContentCatalogJSON, tex)
			if utf8.RuneCountInString(name) > 14 {
				name = string([]rune(name)[:11]) + "…"
			}
			if name != "" {
				dy := float32(11) * z
				if dy < 10 {
					dy = 10
				}
				opts.GeoM.Reset()
				opts.GeoM.Translate(float64(cx+1), float64(cy+dy+1))
				opts.ColorScale.ScaleWithColor(color.RGBA{0x12, 0x14, 0x1c, 0x9a})
				textv2.Draw(screen, name, face, opts)
				opts.GeoM.Reset()
				opts.GeoM.Translate(float64(cx), float64(cy+dy))
				opts.ColorScale.ScaleWithColor(color.RGBA{0xf0, 0xe8, 0xb0, 0xee})
				textv2.Draw(screen, name, face, opts)
			}
		}
	}
}

func (a *App) camZoomEffective() float32 {
	if a.camZoom <= 0 {
		return 1
	}
	return a.camZoom
}

// applyZoomToward оставляет ту же мир-точку под курсором (mx, my) после смены масштаба.
func (a *App) applyZoomToward(mx, my int, newZoom float32) {
	if newZoom < camZoomMin {
		newZoom = camZoomMin
	}
	if newZoom > camZoomMax {
		newZoom = camZoomMax
	}
	z0 := a.camZoomEffective()
	if newZoom == z0 {
		return
	}
	wx := float32(mx)/z0 + a.camX
	wy := float32(my)/z0 + a.camY
	a.camZoom = newZoom
	a.camX = wx - float32(mx)/newZoom
	a.camY = wy - float32(my)/newZoom
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

func (a *App) clampCatItemIdx() {
	n := len(a.catalogInteractIDs)
	if n <= 0 {
		a.catItemIdx = 0
		return
	}
	if a.catItemIdx >= n {
		a.catItemIdx = n - 1
	}
	if a.catItemIdx < 0 {
		a.catItemIdx = 0
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
	if a.pickCatalog {
		if len(a.catalogInteractIDs) == 0 {
			return "wall"
		}
		a.clampCatItemIdx()
		return a.catalogInteractIDs[a.catItemIdx]
	}
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
			a.clampScroll()
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

	if inpututil.IsKeyJustPressed(ebiten.KeyF3) {
		a.showTileOverlay = !a.showTileOverlay
	}

	if a.spawnArgsOpen {
		if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
			a.spawnArgsOpen = false
			a.spawnArgsErr = ""
			return nil
		}
		mx, my := ebiten.CursorPosition()
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			if a.spawnModalCancelHit(mx, my) {
				a.spawnArgsOpen = false
				a.spawnArgsErr = ""
				return nil
			}
			if a.spawnModalOKHit(mx, my) {
				a.confirmSpawnArgsModal()
				return nil
			}
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
			if ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight) {
				a.spawnArgsInsert("\n")
				a.spawnArgsErr = ""
				a.spawnArgsScrollToCursor()
			} else {
				a.confirmSpawnArgsModal()
			}
			return nil
		}
		if ebiten.IsFocused() {
			a.tickSpawnArgsModalInput()
		}
		return nil
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

	pxEdge := a.paletteX()
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) && mx >= pxEdge && mx < pxEdge+paletteSplitterW && my >= 0 && my < a.winH {
		a.paletteDrag = true
		a.paletteDragX = mx
	}
	if !ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		if a.paletteDrag {
			a.paletteDrag = false
		}
	}
	if a.paletteDrag {
		d := mx - a.paletteDragX
		a.paletteDragX = mx
		a.paletteWidth -= d
		a.clampPaletteWidth()
		a.clampScroll()
	}

	a.handlePaletteScroll(mx, my, wx, wy)
	// Колёсико над картой (слева от палитры): масштаб к курсору.
	if pxEdge > 0 && wy != 0 && mx >= 0 && mx < pxEdge && my >= 0 && my < a.winH {
		z0 := a.camZoomEffective()
		f := float32(math.Pow(float64(camZoomWheelStep), -wy))
		a.applyZoomToward(mx, my, z0*f)
	}

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
	} else if a.pickCatalog {
		n := len(a.catalogInteractIDs)
		if n > 0 {
			if inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft) || inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) {
				a.catItemIdx = (a.catItemIdx - 1 + n) % n
			}
			if inpututil.IsKeyJustPressed(ebiten.KeyArrowRight) || inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
				a.catItemIdx = (a.catItemIdx + 1) % n
			}
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
	if pxEdge > 0 {
		if inpututil.IsKeyJustPressed(ebiten.KeyEqual) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadAdd) {
			mxC, myC := pxEdge/2, a.winH/2
			a.applyZoomToward(mxC, myC, a.camZoomEffective()*camZoomWheelStep)
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyMinus) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadSubtract) {
			mxC, myC := pxEdge/2, a.winH/2
			a.applyZoomToward(mxC, myC, a.camZoomEffective()/camZoomWheelStep)
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
	if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonRight) {
		if a.rectEraseDrag {
			a.fillClearRect(a.rectEX0, a.rectEY0, a.rectEX1, a.rectEY1)
			a.rectEraseDrag = false
		}
		a.eraseHave = false
	}

	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		if !a.handlePaletteClick(mx, my) {
			if tx, ty, ok := a.mapTileFromCursor(mx, my); ok {
				if ctrl {
					if !a.pickCatalog {
						a.rectDrag = true
						a.rectX0, a.rectY0 = tx, ty
						a.rectX1, a.rectY1 = tx, ty
					}
				} else if a.pickCatalog {
					a.openSpawnArgsModal(tx, ty)
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
			} else if a.paintHave && !a.pickCatalog {
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
				if ctrl {
					a.rectEraseDrag = true
					a.rectEX0, a.rectEY0 = tx, ty
					a.rectEX1, a.rectEY1 = tx, ty
				} else {
					a.sendClear(tx, ty)
					a.eraseHave = true
					a.eraseNX, a.eraseNY = tx, ty
				}
			}
		}
	}
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) && !inPal {
		if tx, ty, ok := a.mapTileFromCursor(mx, my); ok {
			if ctrl {
				if a.rectEraseDrag {
					a.rectEX1, a.rectEY1 = tx, ty
				}
			} else if a.eraseHave {
				if tx != a.eraseNX || ty != a.eraseNY {
					a.sendClear(tx, ty)
					a.eraseNX, a.eraseNY = tx, ty
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
	camOpts := tiles.DrawOpts{OutlineBlocking: true, CamX: a.camX, CamY: a.camY, CamZoom: a.camZoom}
	for _, t := range tileList {
		tiles.Draw(screen, t, camOpts)
	}
	a.drawEditorTileChrome(screen, tileList)

	if !a.savePanelOpen && !a.spawnArgsOpen {
		cmx, cmy := ebiten.CursorPosition()
		if !a.inPalette(cmx, cmy) {
			if a.rectDrag {
				a.drawTileRectSel(screen, a.rectX0, a.rectY0, a.rectX1, a.rectY1,
					color.RGBA{0x50, 0xa8, 0xf8, 0x22}, color.RGBA{0x88, 0xd0, 0xff, 0xcc})
			} else if a.rectEraseDrag {
				a.drawTileRectSel(screen, a.rectEX0, a.rectEY0, a.rectEX1, a.rectEY1,
					color.RGBA{0xf8, 0x58, 0x50, 0x26}, color.RGBA{0xff, 0x90, 0x88, 0xcc})
			}
			if tx, ty, ok := a.mapTileFromCursor(cmx, cmy); ok && !a.rectEraseDrag {
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
	z := a.camZoomEffective()
	face := &textv2.GoTextFace{Source: ui.FontSource(), Size: world.LabelTextSize * float64(z)}
	for _, id := range ids {
		pl := a.World.Players[id]
		cx, cy := world.ToScreen(pl.X, pl.Y)
		sx := (cx - a.camX) * z
		sy := (cy - a.camY) * z
		pr := world.PlayerRadius * z
		fill := playerColor(id)
		vector.DrawFilledCircle(screen, sx, sy, pr, fill, true)
		stk := float32(1.5) * z
		if stk < 1 {
			stk = 1
		}
		vector.StrokeCircle(screen, sx, sy, pr, stk, color.RGBA{0xff, 0xff, 0xff, 0x90}, true)
		label := fmt.Sprintf("%d", pl.HP)
		opts := &textv2.DrawOptions{}
		opts.PrimaryAlign = textv2.AlignCenter
		opts.SecondaryAlign = textv2.AlignEnd
		gap := world.LabelAboveGap * z
		opts.GeoM.Translate(float64(sx), float64(sy-pr-gap))
		opts.ColorScale.ScaleWithColor(color.RGBA{0xff, 0xff, 0xff, 0xff})
		textv2.Draw(screen, label, face, opts)
	}

	a.drawPalette(screen)

	line1 := "Стрелки — камера · Shift+стрелки — палитра · над картой: колёсико — масштаб · +/- — масштаб к центру · над сеткой палитры: колёсико / Shift+колёсико вбок · ЛКМ кисть · Ctrl+ЛКМ заливка (не «Предметы») · ПКМ стереть · «Предметы»: ЛКМ — окно instance_args · F3 подписи клеток · край палитры — ширина · F2 сохранить · , . слой · R поворот"
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

	if a.spawnArgsOpen {
		a.drawSpawnArgsModal(screen)
	}
	if a.savePanelOpen {
		a.drawSavePanel(screen)
	}
}

const (
	spawnModalBW          = 580
	spawnModalBH          = 320
	spawnModalMaxVisLines = 16
)

func spawnByteToLineCol(s string, b int) (line, col int) {
	if b < 0 {
		return 0, 0
	}
	if b > len(s) {
		b = len(s)
	}
	prefix := s[:b]
	line = strings.Count(prefix, "\n")
	nl := strings.LastIndex(prefix, "\n")
	if nl < 0 {
		col = len(prefix)
	} else {
		col = len(prefix) - nl - 1
	}
	return line, col
}

func spawnLineColToByte(s string, line, col int) int {
	lines := strings.Split(s, "\n")
	if len(lines) == 0 {
		return 0
	}
	if line < 0 {
		line = 0
	}
	if line >= len(lines) {
		line = len(lines) - 1
	}
	off := 0
	for i := 0; i < line; i++ {
		off += len(lines[i]) + 1
	}
	ln := lines[line]
	if col < 0 {
		col = 0
	}
	if col > len(ln) {
		col = len(ln)
	}
	return off + col
}

func spawnCursorByteLeft(s string, b int) int {
	if b <= 0 {
		return 0
	}
	_, sz := utf8.DecodeLastRuneInString(s[:b])
	return b - sz
}

func spawnCursorByteRight(s string, b int) int {
	if b >= len(s) {
		return len(s)
	}
	_, sz := utf8.DecodeRuneInString(s[b:])
	return b + sz
}

func (a *App) spawnModalBox() (bx, by int) {
	bx = (a.winW - spawnModalBW) / 2
	by = (a.winH - spawnModalBH) / 2
	return bx, by
}

func (a *App) spawnModalOKHit(mx, my int) bool {
	bx, by := a.spawnModalBox()
	return inRect(mx, my, bx+spawnModalBW-130, by+spawnModalBH-48, 110, 32)
}

func (a *App) spawnModalCancelHit(mx, my int) bool {
	bx, by := a.spawnModalBox()
	return inRect(mx, my, bx+20, by+spawnModalBH-48, 110, 32)
}

func (a *App) openSpawnArgsModal(tx, ty int) {
	a.spawnArgsPX, a.spawnArgsPY = tx, ty
	a.spawnArgsDraft = gamecontent.EditorInstanceArgsDraft(data.ContentCatalogJSON, a.texture())
	a.spawnArgsErr = ""
	a.spawnArgsOpen = true
	a.spawnArgsCursor = len(a.spawnArgsDraft)
	a.spawnArgsFirstLine = 0
}

func (a *App) confirmSpawnArgsModal() {
	raw, errMsg := gamecontent.ParseAndNormalizeInstanceArgsText(a.spawnArgsDraft)
	if errMsg != "" {
		a.spawnArgsErr = errMsg
		return
	}
	a.sendSpawnCatalog(a.spawnArgsPX, a.spawnArgsPY, raw)
	a.spawnArgsOpen = false
	a.spawnArgsErr = ""
}

func (a *App) clampSpawnArgsCursor() {
	if a.spawnArgsCursor < 0 {
		a.spawnArgsCursor = 0
	}
	if a.spawnArgsCursor > len(a.spawnArgsDraft) {
		a.spawnArgsCursor = len(a.spawnArgsDraft)
	}
}

func (a *App) spawnArgsScrollToCursor() {
	lines := strings.Split(a.spawnArgsDraft, "\n")
	maxFirst := 0
	if len(lines) > spawnModalMaxVisLines {
		maxFirst = len(lines) - spawnModalMaxVisLines
	}
	if a.spawnArgsFirstLine > maxFirst {
		a.spawnArgsFirstLine = maxFirst
	}
	if a.spawnArgsFirstLine < 0 {
		a.spawnArgsFirstLine = 0
	}

	line, _ := spawnByteToLineCol(a.spawnArgsDraft, a.spawnArgsCursor)
	if line < a.spawnArgsFirstLine {
		a.spawnArgsFirstLine = line
	}
	if line >= a.spawnArgsFirstLine+spawnModalMaxVisLines {
		a.spawnArgsFirstLine = line - spawnModalMaxVisLines + 1
	}
	if a.spawnArgsFirstLine < 0 {
		a.spawnArgsFirstLine = 0
	}
}

func (a *App) spawnArgsInsert(s string) {
	a.clampSpawnArgsCursor()
	if len(a.spawnArgsDraft)+len(s) > 120000 {
		return
	}
	a.spawnArgsDraft = a.spawnArgsDraft[:a.spawnArgsCursor] + s + a.spawnArgsDraft[a.spawnArgsCursor:]
	a.spawnArgsCursor += len(s)
}

func (a *App) spawnArgsDeleteForward() {
	a.clampSpawnArgsCursor()
	if a.spawnArgsCursor >= len(a.spawnArgsDraft) {
		return
	}
	_, sz := utf8.DecodeRuneInString(a.spawnArgsDraft[a.spawnArgsCursor:])
	a.spawnArgsDraft = a.spawnArgsDraft[:a.spawnArgsCursor] + a.spawnArgsDraft[a.spawnArgsCursor+sz:]
}

func (a *App) tickSpawnArgsModalInput() {
	a.clampSpawnArgsCursor()

	if inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft) {
		a.spawnArgsCursor = spawnCursorByteLeft(a.spawnArgsDraft, a.spawnArgsCursor)
		a.spawnArgsScrollToCursor()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowRight) {
		a.spawnArgsCursor = spawnCursorByteRight(a.spawnArgsDraft, a.spawnArgsCursor)
		a.spawnArgsScrollToCursor()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) {
		line, col := spawnByteToLineCol(a.spawnArgsDraft, a.spawnArgsCursor)
		if line > 0 {
			a.spawnArgsCursor = spawnLineColToByte(a.spawnArgsDraft, line-1, col)
		}
		a.spawnArgsScrollToCursor()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
		line, col := spawnByteToLineCol(a.spawnArgsDraft, a.spawnArgsCursor)
		lines := strings.Split(a.spawnArgsDraft, "\n")
		if line < len(lines)-1 {
			a.spawnArgsCursor = spawnLineColToByte(a.spawnArgsDraft, line+1, col)
		}
		a.spawnArgsScrollToCursor()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyHome) {
		line, _ := spawnByteToLineCol(a.spawnArgsDraft, a.spawnArgsCursor)
		a.spawnArgsCursor = spawnLineColToByte(a.spawnArgsDraft, line, 0)
		a.spawnArgsScrollToCursor()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnd) {
		line, _ := spawnByteToLineCol(a.spawnArgsDraft, a.spawnArgsCursor)
		lines := strings.Split(a.spawnArgsDraft, "\n")
		endCol := 0
		if line < len(lines) {
			endCol = len(lines[line])
		}
		a.spawnArgsCursor = spawnLineColToByte(a.spawnArgsDraft, line, endCol)
		a.spawnArgsScrollToCursor()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
		if a.spawnArgsCursor > 0 {
			_, sz := utf8.DecodeLastRuneInString(a.spawnArgsDraft[:a.spawnArgsCursor])
			a.spawnArgsDraft = a.spawnArgsDraft[:a.spawnArgsCursor-sz] + a.spawnArgsDraft[a.spawnArgsCursor:]
			a.spawnArgsCursor -= sz
			a.spawnArgsErr = ""
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyDelete) {
		a.spawnArgsDeleteForward()
		a.spawnArgsErr = ""
	}

	for _, ch := range ebiten.AppendInputChars(nil) {
		if ch == '\n' || ch == '\r' {
			continue
		}
		a.spawnArgsInsert(string(ch))
		a.spawnArgsErr = ""
	}
	a.spawnArgsScrollToCursor()
}

func (a *App) drawSpawnArgsModal(screen *ebiten.Image) {
	w, h := float32(a.winW), float32(a.winH)
	vector.DrawFilledRect(screen, 0, 0, w, h, color.RGBA{0x10, 0x10, 0x18, 0xc4}, false)

	ibx, iby := a.spawnModalBox()
	bx, by := float32(ibx), float32(iby)
	bw, bh := float32(spawnModalBW), float32(spawnModalBH)
	vector.DrawFilledRect(screen, bx, by, bw, bh, color.RGBA{0x28, 0x28, 0x34, 0xff}, false)
	vector.StrokeRect(screen, bx, by, bw, bh, 1.5, color.RGBA{0x66, 0x66, 0x80, 0xff}, false)

	face := &textv2.GoTextFace{Source: ui.FontSource(), Size: 15}
	small := &textv2.GoTextFace{Source: ui.FontSource(), Size: 12}
	mono := &textv2.GoTextFace{Source: ui.FontSource(), Size: 11}

	title := fmt.Sprintf("instance_args · (%d,%d) · %s", a.spawnArgsPX, a.spawnArgsPY, a.texture())
	opts := &textv2.DrawOptions{}
	opts.ColorScale.ScaleWithColor(color.RGBA{0xee, 0xee, 0xf5, 0xff})
	opts.GeoM.Translate(float64(bx+14), float64(by+10))
	textv2.Draw(screen, title, face, opts)

	if a.spawnArgsErr != "" {
		em := a.spawnArgsErr
		if utf8.RuneCountInString(em) > 120 {
			em = string([]rune(em)[:117]) + "…"
		}
		opts.GeoM.Reset()
		opts.GeoM.Translate(float64(bx+14), float64(by+36))
		opts.ColorScale.ScaleWithColor(color.RGBA{0xff, 0xa8, 0x88, 0xff})
		textv2.Draw(screen, em, small, opts)
	}

	allLines := strings.Split(a.spawnArgsDraft, "\n")
	fl := a.spawnArgsFirstLine
	if fl < 0 {
		fl = 0
	}
	if fl >= len(allLines) && len(allLines) > 0 {
		fl = len(allLines) - 1
	}
	end := fl + spawnModalMaxVisLines
	if end > len(allLines) {
		end = len(allLines)
	}
	visible := allLines[fl:end]

	var caretX, caretY float32 = -1, -1
	curLine, curCol := spawnByteToLineCol(a.spawnArgsDraft, a.spawnArgsCursor)
	if curLine >= fl && curLine < fl+len(visible) {
		rel := curLine - fl
		ln := allLines[curLine]
		prefix := ln
		if curCol < len(prefix) {
			prefix = prefix[:curCol]
		} else {
			prefix = ln
		}
		w, _ := textv2.Measure(prefix, mono, 0)
		caretX = bx + 14 + float32(w)
		caretY = by + 58 + float32(rel*14)
	}

	ly := by + 58
	for _, ln := range visible {
		opts.GeoM.Reset()
		opts.GeoM.Translate(float64(bx+14), float64(ly))
		opts.ColorScale.ScaleWithColor(color.RGBA{0xd8, 0xdc, 0xec, 0xff})
		textv2.Draw(screen, ln, mono, opts)
		ly += 14
	}
	if caretX >= 0 {
		vector.DrawFilledRect(screen, caretX, caretY, 2, 14, color.RGBA{0xff, 0xe8, 0x88, 0xee}, false)
	}

	vector.DrawFilledRect(screen, bx+20, by+bh-48, 110, 32, color.RGBA{0x40, 0x40, 0x50, 0xff}, false)
	vector.DrawFilledRect(screen, bx+bw-130, by+bh-48, 110, 32, color.RGBA{0x38, 0x58, 0x78, 0xff}, false)
	opts = &textv2.DrawOptions{}
	opts.PrimaryAlign = textv2.AlignCenter
	opts.SecondaryAlign = textv2.AlignCenter
	opts.GeoM.Translate(float64(bx+75), float64(by+bh-32))
	opts.ColorScale.ScaleWithColor(color.RGBA{0xe8, 0xe8, 0xf0, 0xff})
	textv2.Draw(screen, "Отмена", small, opts)
	opts.GeoM.Reset()
	opts.GeoM.Translate(float64(bx+bw-75), float64(by+bh-32))
	textv2.Draw(screen, "OK", small, opts)

	opts.GeoM.Reset()
	opts.GeoM.Translate(float64(bx+14), float64(by+bh-22))
	opts.PrimaryAlign = textv2.AlignStart
	opts.ColorScale.ScaleWithColor(color.RGBA{0x88, 0x8c, 0x9c, 0xff})
	textv2.Draw(screen, "←→↑↓ Home/End · Backspace/Delete · Shift+Enter — строка · Enter — OK · Esc — отмена · до 64 KiB", small, opts)
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
	a.clampPaletteWidth()
	a.clampScroll()
	return outsideWidth, outsideHeight
}
