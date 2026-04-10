package tiles

import (
	"bytes"
	"image/color"
	"image/png"
	"slices"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/rudolfkova/grpc_auth/pkg/gamekit"

	"client/data"
	"client/internal/world"
)

// Имена текстур на wire (state / spawn_tile) → файлы в data/assets.
var assetFiles = map[string]string{
	"grass": "Grass_Middle.png",
	"water": "Water_Middle.png",
	"path":  "Path_Middle.png",
}

var (
	imgMu   sync.Mutex
	imgByID = make(map[string]*ebiten.Image)
)

// ImageForTexture картинка для ключа (тайлсет или grass/water/path); nil если неизвестно.
func ImageForTexture(name string) *ebiten.Image {
	return imageForTexture(name)
}

// EditorSingleTextureKeys отдельные текстуры из assets/tiles (wire-ключи), по алфавиту.
func EditorSingleTextureKeys() []string {
	keys := make([]string, 0, len(assetFiles))
	for k := range assetFiles {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func imageForTexture(name string) *ebiten.Image {
	imgMu.Lock()
	if img, ok := imgByID[name]; ok {
		imgMu.Unlock()
		return img
	}
	imgMu.Unlock()

	// Тайлсет: Beach_Tile_7 → лениво грузим весь лист, если init ещё не успел.
	if base, _, ok := ParseIndexedTexture(name); ok {
		ensureTilesetLoaded(base)
		imgMu.Lock()
		img := imgByID[name]
		imgMu.Unlock()
		if img != nil {
			return img
		}
	}

	imgMu.Lock()
	defer imgMu.Unlock()
	if img, ok := imgByID[name]; ok {
		return img
	}
	file, ok := assetFiles[name]
	if !ok {
		return nil
	}
	raw, err := data.TileAssets.ReadFile("assets/" + file)
	if err != nil {
		return nil
	}
	decoded, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil
	}
	img := ebiten.NewImageFromImage(decoded)
	imgByID[name] = img
	return img
}

// DrawOpts настройки отрисовки тайла на экране.
type DrawOpts struct {
	// OutlineBlocking — красная рамка для клеток с коллизией (редактор).
	OutlineBlocking bool
}

// Draw рисует один тайл: текстура из assets либо сплошная заливка-заглушка.
func Draw(screen *ebiten.Image, t gamekit.Tile, opts DrawOpts) {
	x0 := float32(world.GridPad) + float32(t.X)*float32(world.TileSize)
	y0 := float32(world.GridPad) + float32(t.Y)*float32(world.TileSize)
	ts := float32(world.TileSize)

	img := imageForTexture(t.Texture)
	if img != nil {
		w, h := img.Bounds().Dx(), img.Bounds().Dy()
		op := &ebiten.DrawImageOptions{}
		if w > 0 && h > 0 {
			op.GeoM.Scale(float64(ts)/float64(w), float64(ts)/float64(h))
		}
		op.GeoM.Translate(float64(x0), float64(y0))
		screen.DrawImage(img, op)
	} else {
		fill := color.RGBA{0x44, 0x44, 0x55, 0xee}
		if !t.Blocks {
			fill = color.RGBA{0x28, 0x50, 0x38, 0xbb}
		}
		vector.DrawFilledRect(screen, x0, y0, ts, ts, fill, false)
	}

	if opts.OutlineBlocking && t.Blocks {
		const stroke float32 = 2
		vector.StrokeRect(screen, x0, y0, ts, ts, stroke, color.RGBA{0xe0, 0x30, 0x30, 0xff}, false)
	}
}
