package playeranim

import (
	"bytes"
	"image/png"
	"log"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"

	"client/data"
)

const Female012SheetPath = "Female 01-2/Female 01-2.png"

var female012Once sync.Once
var female012Sheet *ebiten.Image

func female012Load() {
	raw, err := data.FemaleCharAssets.ReadFile(Female012SheetPath)
	if err != nil {
		log.Printf("playeranim: %s: %v", Female012SheetPath, err)
		return
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		log.Printf("playeranim: decode %s: %v", Female012SheetPath, err)
		return
	}
	female012Sheet = ebiten.NewImageFromImage(img)
}

func Female012Sheet() *ebiten.Image {
	female012Once.Do(female012Load)
	return female012Sheet
}

// DrawFemale012 — тот же расклад кадров, что у Male 01-1 (32×32, 4 ряда по сторонам).
func DrawFemale012(dst *ebiten.Image, cx, cy float32, cardinal int, walking bool, phase float64, scale float64) {
	drawWalkSheet(dst, Female012Sheet(), cx, cy, cardinal, walking, phase, scale)
}
