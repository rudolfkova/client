package main

import (
	_ "embed"
	"image/color"
	"log"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

//go:embed shaders/fog.kage
var fogKage []byte

const tileSize = 32

// fogTileOmittedForTest — часть плиток намеренно не рисуем: проступает фон,
// по оставшимся видно, что туман — одна непрерывная «форма», а не заливка по клеткам.
func fogTileOmittedForTest(tx, ty, w, h int) bool {
	ix := tx / tileSize
	iy := ty / tileSize
	nx := (w + tileSize - 1) / tileSize
	ny := (h + tileSize - 1) / tileSize

	dx := ix - nx/2
	dy := iy - ny/2
	// круглая дырка в центре (в координатах плиток)
	if dx*dx+dy*dy < 28 {
		return true
	}
	// «рваные» пропуски по полю
	if (ix*13+iy*7)%11 < 3 {
		return true
	}
	return false
}

type app struct {
	shader *ebiten.Shader
	start  time.Time
}

func (a *app) Update() error {
	return nil
}

func (a *app) Draw(screen *ebiten.Image) {
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()
	screen.Fill(color.RGBA{R: 0x18, G: 0x1c, B: 0x28, A: 0xff})

	t := float32(time.Since(a.start).Seconds())

	for ty := 0; ty < h; ty += tileSize {
		th := tileSize
		if ty+th > h {
			th = h - ty
		}
		for tx := 0; tx < w; tx += tileSize {
			tw := tileSize
			if tx+tw > w {
				tw = w - tx
			}
			if tw <= 0 || th <= 0 {
				continue
			}
			if fogTileOmittedForTest(tx, ty, w, h) {
				continue
			}

			var gm ebiten.GeoM
			gm.Translate(float64(tx), float64(ty))
			op := &ebiten.DrawRectShaderOptions{
				GeoM: gm,
				Uniforms: map[string]any{
					"Time":     t,
					"EdgeFade": float32(1),
					"CamX":     float32(0),
					"CamY":     float32(0),
				},
			}
			screen.DrawRectShader(tw, th, a.shader, op)
		}
	}
}

func (a *app) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func main() {
	sh, err := ebiten.NewShader(fogKage)
	if err != nil {
		log.Fatalf("shader: %v", err)
	}
	ebiten.SetWindowTitle("fog — 32px tiles, часть вырезана (тест)")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowSize(960, 540)
	if err := ebiten.RunGame(&app{shader: sh, start: time.Now()}); err != nil {
		log.Fatal(err)
	}
}
