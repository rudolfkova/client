package main

import (
	"bytes"
	_ "embed"
	"image"
	_ "image/png"
	"log"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

//go:embed shaders/fog.kage
var fogKage []byte

//go:embed shaders/blur.kage
var blurKage []byte

//go:embed "shaders/sample 1.png"
var samplePNG []byte

const (
	tileSize         = 32
	blurStrengthPx   = 5.0
	blurPadExtra int = 4 // запас к радиусу сэмплов в blur.kage
)

type app struct {
	source   *ebiten.Image
	scratch  *ebiten.Image
	blurred  *ebiten.Image
	haloIn   *ebiten.Image
	haloOut  *ebiten.Image
	blurSh   *ebiten.Shader
	fogSh    *ebiten.Shader
	bufW     int
	bufH     int
	haloCW   int
	haloCH   int
	start    time.Time
}

func blurPad() int {
	return int(math.Ceil(blurStrengthPx)) + blurPadExtra
}

func (a *app) ensureBuffers(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	if a.scratch != nil && a.bufW == w && a.bufH == h {
		return
	}
	a.bufW, a.bufH = w, h
	a.scratch = ebiten.NewImage(w, h)
	a.blurred = ebiten.NewImage(w, h)

	pad := blurPad()
	a.haloCW = min(w, tileSize+2*pad)
	a.haloCH = min(h, tileSize+2*pad)
	a.haloIn = ebiten.NewImage(a.haloCW, a.haloCH)
	a.haloOut = ebiten.NewImage(a.haloCW, a.haloCH)
}

// drawImageCover масштабирует src так, чтобы заполнить весь dst (обрезка по краям как object-fit: cover).
func drawImageCover(dst *ebiten.Image, src *ebiten.Image) {
	dw, dh := dst.Bounds().Dx(), dst.Bounds().Dy()
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	if sw <= 0 || sh <= 0 {
		return
	}
	scale := max(float64(dw)/float64(sw), float64(dh)/float64(sh))
	nw := float64(sw) * scale
	nh := float64(sh) * scale
	tx := (float64(dw) - nw) / 2
	ty := (float64(dh) - nh) / 2

	var op ebiten.DrawImageOptions
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(tx, ty)
	dst.DrawImage(src, &op)
}

// drawBlurTiledPerTile непрерывный блюр при разбиении на тайлы: для каждого тайла в scratch
// копируется окно «тайл + ореол» (halo), чтобы ядро могло сэмплить соседей; в blurred попадает только центр.
func (a *app) drawBlurTiledPerTile() {
	w, h := a.bufW, a.bufH
	pad := blurPad()

	a.blurred.Clear()

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

			ax := max(0, tx-pad)
			ay := max(0, ty-pad)
			bx := min(w, tx+tw+pad)
			by := min(h, ty+th+pad)
			cw := bx - ax
			ch := by - ay
			if cw <= 0 || ch <= 0 {
				continue
			}

			subIn := a.haloIn.SubImage(image.Rect(0, 0, cw, ch)).(*ebiten.Image)
			subOut := a.haloOut.SubImage(image.Rect(0, 0, cw, ch)).(*ebiten.Image)

			subIn.Clear()
			var cop ebiten.DrawImageOptions
			cop.GeoM.Translate(-float64(ax), -float64(ay))
			subIn.DrawImage(a.scratch, &cop)

			bop := &ebiten.DrawRectShaderOptions{
				Uniforms: map[string]any{
					"BlurPx": float32(blurStrengthPx),
				},
				Images: [4]*ebiten.Image{},
			}
			bop.Images[0] = subIn
			subOut.DrawRectShader(cw, ch, a.blurSh, bop)

			ox := tx - ax
			oy := ty - ay
			tileFrom := subOut.SubImage(image.Rect(ox, oy, ox+tw, oy+th)).(*ebiten.Image)
			var dop ebiten.DrawImageOptions
			dop.GeoM.Translate(float64(tx), float64(ty))
			a.blurred.DrawImage(tileFrom, &dop)
		}
	}
}

func (a *app) Update() error {
	return nil
}

func (a *app) Draw(screen *ebiten.Image) {
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()
	a.ensureBuffers(w, h)
	if a.scratch == nil {
		return
	}

	drawImageCover(a.scratch, a.source)
	a.drawBlurTiledPerTile()

	screen.DrawImage(a.blurred, nil)

	t := float32(time.Since(a.start).Seconds())
	fop := &ebiten.DrawRectShaderOptions{
		Uniforms: map[string]any{
			"Time":     t,
			"EdgeFade": float32(1),
			"CamX":     float32(0),
			"CamY":     float32(0),
		},
	}
	screen.DrawRectShader(w, h, a.fogSh, fop)
}

func (a *app) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func main() {
	img, _, err := image.Decode(bytes.NewReader(samplePNG))
	if err != nil {
		log.Fatalf("png: %v", err)
	}
	source := ebiten.NewImageFromImage(img)

	blurSh, err := ebiten.NewShader(blurKage)
	if err != nil {
		log.Fatalf("blur shader: %v", err)
	}
	fogSh, err := ebiten.NewShader(fogKage)
	if err != nil {
		log.Fatalf("fog shader: %v", err)
	}

	ebiten.SetWindowTitle("fog example — sample → blur (tiled+halo) → fog")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowSize(960, 540)
	if err := ebiten.RunGame(&app{
		source: source,
		blurSh: blurSh,
		fogSh:  fogSh,
		start:  time.Now(),
	}); err != nil {
		log.Fatal(err)
	}
}
