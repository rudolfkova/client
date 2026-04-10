package editor

import (
	"bytes"
	"image/color"
	"image/png"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/rudolfkova/grpc_auth/pkg/gamekit"

	"client/data"
	"client/internal/world"
)

// Имена текстур на wire (spawn_tile / state) → файлы в data/assets.
var textureAssetFiles = map[string]string{
	"grass": "Grass_Middle.png",
	"water": "Water_Middle.png",
	"path":  "Path_Middle.png",
}

var (
	tileImageMu   sync.Mutex
	tileImageByID = make(map[string]*ebiten.Image)
)

func tileImageForTexture(name string) *ebiten.Image {
	tileImageMu.Lock()
	defer tileImageMu.Unlock()
	if img, ok := tileImageByID[name]; ok {
		return img
	}
	file, ok := textureAssetFiles[name]
	if !ok {
		tileImageByID[name] = nil
		return nil
	}
	raw, err := data.TileAssets.ReadFile("assets/" + file)
	if err != nil {
		tileImageByID[name] = nil
		return nil
	}
	decoded, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		tileImageByID[name] = nil
		return nil
	}
	img := ebiten.NewImageFromImage(decoded)
	tileImageByID[name] = img
	return img
}

func tileTopLeft(tileX, tileY int) (x0, y0 float32) {
	x0 = float32(world.GridPad) + float32(tileX)*float32(world.TileSize)
	y0 = float32(world.GridPad) + float32(tileY)*float32(world.TileSize)
	return x0, y0
}

func drawTile(screen *ebiten.Image, t gamekit.Tile) {
	x0, y0 := tileTopLeft(t.X, t.Y)
	ts := float32(world.TileSize)

	img := tileImageForTexture(t.Texture)
	if img != nil {
		w, h := img.Bounds().Dx(), img.Bounds().Dy()
		op := &ebiten.DrawImageOptions{}
		if w > 0 && h > 0 {
			op.GeoM.Scale(float64(ts)/float64(w), float64(ts)/float64(h))
		}
		op.GeoM.Translate(float64(x0), float64(y0))
		screen.DrawImage(img, op)
	} else {
		// Нет файла текстуры (например wall) — сплошная заливка без окраса от коллизии.
		fill := color.RGBA{0x44, 0x44, 0x55, 0xee}
		if !t.Blocks {
			fill = color.RGBA{0x28, 0x50, 0x38, 0xbb}
		}
		vector.DrawFilledRect(screen, x0, y0, ts, ts, fill, false)
	}

	if t.Blocks {
		const stroke float32 = 2
		vector.StrokeRect(screen, x0, y0, ts, ts, stroke, color.RGBA{0xe0, 0x30, 0x30, 0xff}, false)
	}
}
