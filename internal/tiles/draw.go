package tiles

import (
	"bytes"
	"image/color"
	"image/png"
	"math"
	"slices"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/rudolfkova/grpc_auth/pkg/gamekit"

	"client/data"
	"client/internal/world"
)

var (
	imgMu   sync.Mutex
	imgByID = make(map[string]*ebiten.Image)
)

// ImageForTexture картинка для ключа (тайлсет Base_N или одиночный PNG из корня assets/); nil если неизвестно.
func ImageForTexture(name string) *ebiten.Image {
	return imageForTexture(name)
}

// EditorSingleTextureKeys отдельные PNG из корня assets/ (не tileSets), ключ = имя файла без .png.
func EditorSingleTextureKeys() []string {
	keys := make([]string, 0, len(singleTextureFiles))
	for k := range singleTextureFiles {
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
	path, ok := singleTextureFiles[name]
	if !ok {
		return nil
	}
	raw, err := data.TileAssets.ReadFile(path)
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
		DrawTextureScaledRotated(screen, img, x0+ts/2, y0+ts/2, ts, t.Rotation)
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

// DrawTextureScaledRotated — центр (cx, cy), сторона квадрата size, четверти по часовой (как на сервере).
func DrawTextureScaledRotated(dst *ebiten.Image, img *ebiten.Image, cx, cy, size float32, rotationQuarter int) {
	if img == nil {
		return
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if w <= 0 || h <= 0 {
		return
	}
	q := NormalizeRotationQuarter(rotationQuarter)
	op := &ebiten.DrawImageOptions{}
	sx := float64(size) / float64(w)
	sy := float64(size) / float64(h)
	op.GeoM.Translate(-float64(w)/2, -float64(h)/2)
	op.GeoM.Scale(sx, sy)
	op.GeoM.Rotate(float64(q) * math.Pi / 2)
	op.GeoM.Translate(float64(cx), float64(cy))
	dst.DrawImage(img, op)
}
