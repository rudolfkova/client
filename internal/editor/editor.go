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
	"client/internal/ui"
	"client/internal/world"
)

const (
	WindowWidth  = 1600
	WindowHeight = 800
)

var brushTextures = []string{"wall", "grass", "water", "path"}

// App — клиент редактора мира: spawn_tile по клику, превью состояния с game WS.
type App struct {
	wsGame *websocket.Conn
	msgs   <-chan gamekit.Envelope
	World  *state.World

	brushIndex int
	blocks     bool
}

func New(wsGame *websocket.Conn, msgs <-chan gamekit.Envelope) *App {
	_ = ui.FontSource()
	return &App{
		wsGame:     wsGame,
		msgs:       msgs,
		World:      state.NewWorld(),
		brushIndex: 0,
		blocks:     true,
	}
}

func (a *App) texture() string {
	if len(brushTextures) == 0 {
		return "wall"
	}
	return brushTextures[a.brushIndex%len(brushTextures)]
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
			if inpututil.IsKeyJustPressed(ebiten.KeyB) {
				a.blocks = !a.blocks
			}
			for i := range brushTextures {
				key := ebiten.KeyDigit1 + ebiten.Key(i)
				if inpututil.IsKeyJustPressed(key) {
					a.brushIndex = i
				}
			}
			if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
				mx, my := ebiten.CursorPosition()
				tx, ty, ok := world.TileFromScreen(mx, my)
				if ok {
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
			return nil
		}
	}
}

func (a *App) Draw(screen *ebiten.Image) {
	screen.Clear()

	tiles := slices.Clone(a.World.Tiles)
	slices.SortFunc(tiles, func(x, y gamekit.Tile) int {
		if x.Y != y.Y {
			return x.Y - y.Y
		}
		return x.X - y.X
	})
	for _, t := range tiles {
		drawTile(screen, t)
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

	line1 := fmt.Sprintf("Редактор — ЛКМ: поставить тайл   1-%d: текстура   B: коллизия %v",
		len(brushTextures), a.blocks)
	line2 := fmt.Sprintf("Текущая текстура: %q", a.texture())
	hudFace := &textv2.GoTextFace{Source: ui.FontSource(), Size: 14}
	hudOpts := &textv2.DrawOptions{}
	hudOpts.ColorScale.ScaleWithColor(color.RGBA{0xf0, 0xf0, 0xf0, 0xff})
	hudOpts.GeoM.Translate(12, 8)
	textv2.Draw(screen, line1, hudFace, hudOpts)
	hudOpts.GeoM.Reset()
	hudOpts.GeoM.Translate(12, 26)
	textv2.Draw(screen, line2, hudFace, hudOpts)
}

func playerColor(id int64) color.RGBA {
	ii := int(id)
	h := uint8(37*ii + 80)
	return color.RGBA{R: h, G: uint8(200 - (ii*17)%80), B: uint8(120 + (ii*13)%100), A: 0xff}
}

func (a *App) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}
