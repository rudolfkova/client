package playeranim

import (
	"bytes"
	"image"
	"image/png"
	"log"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"

	"client/data"
)

const Male01SheetPath = "anim/Male 01-1/Male 01-1.png"

var male01Once sync.Once
var male01Sheet *ebiten.Image

func male01Load() {
	raw, err := data.AnimAssets.ReadFile(Male01SheetPath)
	if err != nil {
		log.Printf("playeranim: %s: %v", Male01SheetPath, err)
		return
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		log.Printf("playeranim: decode %s: %v", Male01SheetPath, err)
		return
	}
	male01Sheet = ebiten.NewImageFromImage(img)
}

// Male01Sheet загружает лист один раз; при ошибке nil.
func Male01Sheet() *ebiten.Image {
	male01Once.Do(male01Load)
	return male01Sheet
}

// WalkSpriteRow: строка листа — ряд 0 вниз, 1 влево, 2 вправо, 3 вверх.
func WalkSpriteRow(cardinal int) int {
	switch cardinal {
	case CardinalS:
		return 0
	case CardinalW:
		return 1
	case CardinalE:
		return 2
	case CardinalN:
		return 3
	default:
		return 2
	}
}

// WalkFrameCol: стоит — средний кадр (1); идёт — цикл 0→1→2→1.
func WalkFrameCol(walking bool, phase float64) int {
	if !walking {
		return 1
	}
	i := int(phase) % 4
	return [...]int{0, 1, 2, 1}[i]
}

// DrawMale01 рисует кадр по центру (cx, cy); scale — множитель к размеру кадра.
func DrawMale01(dst *ebiten.Image, cx, cy float32, cardinal int, walking bool, phase float64, scale float64) {
	drawWalkSheet(dst, Male01Sheet(), cx, cy, cardinal, walking, phase, scale)
}

func drawWalkSheet(dst *ebiten.Image, img *ebiten.Image, cx, cy float32, cardinal int, walking bool, phase float64, scale float64) {
	if img == nil {
		return
	}
	row := WalkSpriteRow(cardinal)
	col := WalkFrameCol(walking, phase)
	sx0 := col * WalkFramePx
	sy0 := row * WalkFramePx
	sub := img.SubImage(image.Rect(sx0, sy0, sx0+WalkFramePx, sy0+WalkFramePx)).(*ebiten.Image)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(-float64(WalkFramePx)/2, -float64(WalkFramePx)/2)
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(float64(cx), float64(cy))
	dst.DrawImage(sub, op)
}
