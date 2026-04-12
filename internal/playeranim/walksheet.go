package playeranim

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"log"
	"strings"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"

	"client/data"
)

var walkSheetMu sync.Mutex
var walkSheetCache = map[string]*ebiten.Image{}

// SanitizeSpriteID отсекает опасные пути; пустая строка — невалидно.
func SanitizeSpriteID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.Contains(s, "..") || strings.ContainsAny(s, `/\`) {
		return ""
	}
	return s
}

func walkEmbedPath(spriteID string) string {
	return fmt.Sprintf("anim/%s/%s.png", spriteID, spriteID)
}

// WalkSheet загружает лист ходьбы из embed data/anim/<id>/<id>.png (кэш по id).
func WalkSheet(spriteID string) *ebiten.Image {
	id := SanitizeSpriteID(spriteID)
	if id == "" {
		return nil
	}
	walkSheetMu.Lock()
	defer walkSheetMu.Unlock()
	if img, ok := walkSheetCache[id]; ok {
		return img
	}
	path := walkEmbedPath(id)
	raw, err := data.AnimAssets.ReadFile(path)
	if err != nil {
		log.Printf("playeranim: %s: %v", path, err)
		return nil
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		log.Printf("playeranim: decode %s: %v", path, err)
		return nil
	}
	eb := ebiten.NewImageFromImage(img)
	walkSheetCache[id] = eb
	return eb
}

// DrawWalkSheet рисует кадр листа spriteID (тот же расклад, что Male 01-1: 32×32, 4 ряда по сторонам света).
func DrawWalkSheet(dst *ebiten.Image, spriteID string, cx, cy float32, cardinal int, walking bool, phase float64, scale float64) {
	drawWalkSheet(dst, WalkSheet(spriteID), cx, cy, cardinal, walking, phase, scale)
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
