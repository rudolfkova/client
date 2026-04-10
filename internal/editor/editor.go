package editor

import (
	"fmt"
	"image/color"
	"log"
	"slices"

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

	pickTilesets  bool // true: тайлсеты, false: grass/water/path
	singleIdx     int
	paletteScroll int
	winW, winH    int
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
	}
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
	for {
		select {
		case msg, ok := <-a.msgs:
			if !ok {
				return fmt.Errorf("websocket closed")
			}
			a.World.ApplyEnvelope(msg)
		default:
			mx, my := ebiten.CursorPosition()
			_, wy := ebiten.Wheel()
			a.handlePaletteScroll(mx, my, wy)

			if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
				a.blocks = !a.blocks
			}
			if a.pickTilesets {
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
					a.paletteScroll = 0
				}
				if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
					a.nextSet(1)
					a.paletteScroll = 0
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
			if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
				if !a.handlePaletteClick(mx, my) {
					if tx, ty, ok := a.mapTileFromCursor(mx, my); ok {
						intent := gamekit.TileSpawnIntent{
							X:       tx,
							Y:       ty,
							Texture: a.texture(),
							Blocks:  a.blocks,
						}
						if err := gamews.Send(a.wsGame, gamekit.TypeSpawnTile, intent); err != nil {
							log.Printf("editor spawn_tile: %v", err)
						}
					}
				}
			}
			return nil
		}
	}
}

func (a *App) Draw(screen *ebiten.Image) {
	screen.Clear()

	tileList := slices.Clone(a.World.Tiles)
	slices.SortFunc(tileList, func(x, y gamekit.Tile) int {
		if x.Y != y.Y {
			return x.Y - y.Y
		}
		return x.X - y.X
	})
	for _, t := range tileList {
		tiles.Draw(screen, t, tiles.DrawOpts{OutlineBlocking: true})
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

	line1 := "ЛКМ на поле — тайл · Пробел — коллизия · Палитра справа: сетка, вкладки, предпросмотр"
	hudFace := &textv2.GoTextFace{Source: ui.FontSource(), Size: 14}
	hudOpts := &textv2.DrawOptions{}
	hudOpts.ColorScale.ScaleWithColor(color.RGBA{0xf0, 0xf0, 0xf0, 0xff})
	hudOpts.GeoM.Translate(12, 8)
	textv2.Draw(screen, line1, hudFace, hudOpts)
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
