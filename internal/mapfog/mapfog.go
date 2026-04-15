package mapfog

import (
	_ "embed"
	"image/color"
	"log"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/rudolfkova/grpc_auth/pkg/gamekit"

	"client/internal/world"
)

//go:embed fog.kage
var fogKage []byte

//go:embed blur.kage
var blurKage []byte

//go:embed composite.kage
var compositeKage []byte

const (
	// blurStrengthPx — полноэкранный блюр (маска смягчает края клеток).
	blurStrengthPx = float32(3)
)

// RenderMode — что рисовать по пустым клеткам слоя 0. Цикл: All → FogOnly → BlurOnly → None.
type RenderMode uint8

const (
	RenderBlurAndFog RenderMode = iota // блюр + туман
	RenderFogOnly                      // только туман
	RenderBlurOnly                     // только блюр
	RenderNone                         // выключено
)

// NextRenderMode следующий режим по кругу (для горячей клавиши).
func NextRenderMode(m RenderMode) RenderMode {
	return (m + 1) % 4
}

type voidCell struct {
	rx0, ry0, tw, th int
	edgeFade         float32
}

// Fog процедурный туман по пустым клеткам слоя 0 (см. example/fog/FOG_SHADER.md).
// Блюр: один проход по всему экрану + маска пустых клеток + composite (дешевле сотен тайловых шейдеров).
type Fog struct {
	sh         *ebiten.Shader
	blurSh     *ebiten.Shader
	compositeSh *ebiten.Shader
	scratch    *ebiten.Image
	blurred    *ebiten.Image
	mask       *ebiten.Image
	bufW       int
	bufH       int
	voidBuf    []voidCell
	start      time.Time
}

// NewFog компилирует шейдеры; при ошибке соответствующий указатель nil.
func NewFog() *Fog {
	f := &Fog{start: time.Now()}
	if sh, err := ebiten.NewShader(fogKage); err != nil {
		log.Printf("mapfog: fog shader: %v", err)
	} else {
		f.sh = sh
	}
	if bsh, err := ebiten.NewShader(blurKage); err != nil {
		log.Printf("mapfog: blur shader: %v", err)
	} else {
		f.blurSh = bsh
	}
	if csh, err := ebiten.NewShader(compositeKage); err != nil {
		log.Printf("mapfog: composite shader: %v", err)
	} else {
		f.compositeSh = csh
	}
	return f
}

func (f *Fog) ensureBuffers(dst *ebiten.Image) {
	if f == nil || f.blurSh == nil || f.compositeSh == nil {
		return
	}
	ww, wh := dst.Bounds().Dx(), dst.Bounds().Dy()
	if ww <= 0 || wh <= 0 {
		return
	}
	if f.scratch != nil && f.bufW == ww && f.bufH == wh {
		return
	}
	f.bufW, f.bufH = ww, wh
	f.scratch = ebiten.NewImage(ww, wh)
	f.blurred = ebiten.NewImage(ww, wh)
	f.mask = ebiten.NewImage(ww, wh)
}

func (f *Fog) blurComposite(dst *ebiten.Image, cells []voidCell, ww, wh int) {
	if f.blurSh == nil || f.compositeSh == nil || f.scratch == nil || f.blurred == nil || f.mask == nil {
		return
	}
	f.scratch.DrawImage(dst, nil)

	bop := &ebiten.DrawRectShaderOptions{
		Uniforms: map[string]any{
			"BlurPx": blurStrengthPx,
		},
		Images: [4]*ebiten.Image{f.scratch},
	}
	f.blurred.DrawRectShader(ww, wh, f.blurSh, bop)

	f.mask.Clear()
	for i := range cells {
		c := &cells[i]
		a := uint8(math.Round(float64(c.edgeFade) * 255))
		if a == 0 {
			continue
		}
		vector.DrawFilledRect(f.mask, float32(c.rx0), float32(c.ry0), float32(c.tw), float32(c.th),
			color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: a}, false)
	}

	cop := &ebiten.DrawRectShaderOptions{
		Images: [4]*ebiten.Image{f.scratch, f.blurred, f.mask},
	}
	dst.DrawRectShader(ww, wh, f.compositeSh, cop)
}

type tileXY struct{ x, y int }

// nearestLayer0Cheb расстояние Чебышёва до ближайшей клетки со слоем 0 в окне ±searchR (иначе searchR+1).
func nearestLayer0Cheb(tx, ty int, occ map[tileXY]struct{}, searchR int) int {
	if len(occ) == 0 {
		return searchR + 1
	}
	best := searchR + 1
	for dy := -searchR; dy <= searchR; dy++ {
		for dx := -searchR; dx <= searchR; dx++ {
			if _, ok := occ[tileXY{tx + dx, ty + dy}]; !ok {
				continue
			}
			d := max(abs(dx), abs(dy))
			if d < best {
				best = d
			}
		}
	}
	return best
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// fadeFromBorderDist плавное нарастание тумана от границы с землёй (d — расстояние в клетках, d≥1 у пустых у кромки).
func fadeFromBorderDist(d int) float32 {
	const edge0, edge1 = 0.35, 4.0
	if d <= 0 {
		return 0
	}
	fd := float64(d)
	if fd >= edge1 {
		return 1
	}
	if fd <= edge0 {
		return 0
	}
	t := (fd - edge0) / (edge1 - edge0)
	// smoothstep
	return float32(t * t * (3 - 2*t))
}

// Draw поверх dst: блюр по маске пустых клеток (один полноэкранный проход), затем туман по клеткам — согласно mode.
func (f *Fog) Draw(dst *ebiten.Image, tiles []gamekit.Tile, camX, camY float32, mode RenderMode) {
	if f == nil || mode == RenderNone {
		return
	}
	wantBlur := mode == RenderBlurAndFog || mode == RenderBlurOnly
	wantFog := mode == RenderBlurAndFog || mode == RenderFogOnly
	if wantBlur && (f.blurSh == nil || f.compositeSh == nil) {
		wantBlur = false
	}
	if wantFog && f.sh == nil {
		wantFog = false
	}
	if !wantBlur && !wantFog {
		return
	}
	ww, wh := dst.Bounds().Dx(), dst.Bounds().Dy()
	if ww <= 0 || wh <= 0 {
		return
	}

	occ := make(map[tileXY]struct{}, 64)
	for i := range tiles {
		t := &tiles[i]
		if t.Layer != 0 {
			continue
		}
		occ[tileXY{t.X, t.Y}] = struct{}{}
	}

	minTX, minTY, maxTX, maxTY := visibleTileRange(ww, wh, camX, camY)
	t := float32(time.Since(f.start).Seconds())
	ts := float64(world.TileSize)
	pad := float64(world.GridPad)
	const searchR = 12

	f.voidBuf = f.voidBuf[:0]
	for ty := minTY; ty <= maxTY; ty++ {
		for tx := minTX; tx <= maxTX; tx++ {
			if _, ok := occ[tileXY{tx, ty}]; ok {
				continue
			}
			d := nearestLayer0Cheb(tx, ty, occ, searchR)
			edgeFade := fadeFromBorderDist(d)
			if edgeFade <= 0.001 {
				continue
			}
			sx := pad + float64(tx)*ts - float64(camX)
			sy := pad + float64(ty)*ts - float64(camY)
			ix0 := int(math.Floor(sx))
			iy0 := int(math.Floor(sy))
			ix1 := ix0 + world.TileSize
			iy1 := iy0 + world.TileSize
			rx0 := max(0, ix0)
			ry0 := max(0, iy0)
			rx1 := min(ww, ix1)
			ry1 := min(wh, iy1)
			tw := rx1 - rx0
			th := ry1 - ry0
			if tw <= 0 || th <= 0 {
				continue
			}
			f.voidBuf = append(f.voidBuf, voidCell{rx0: rx0, ry0: ry0, tw: tw, th: th, edgeFade: edgeFade})
		}
	}

	if wantBlur && len(f.voidBuf) > 0 {
		f.ensureBuffers(dst)
		f.blurComposite(dst, f.voidBuf, ww, wh)
	}

	if !wantFog {
		return
	}
	for i := range f.voidBuf {
		c := &f.voidBuf[i]
		var gm ebiten.GeoM
		gm.Translate(float64(c.rx0), float64(c.ry0))
		op := &ebiten.DrawRectShaderOptions{
			GeoM: gm,
			Uniforms: map[string]any{
				"Time":     t,
				"EdgeFade": c.edgeFade,
				"CamX":     camX,
				"CamY":     camY,
			},
		}
		dst.DrawRectShader(c.tw, c.th, f.sh, op)
	}
}

func visibleTileRange(ww, wh int, camX, camY float32) (minTX, minTY, maxTX, maxTY int) {
	const margin = 1
	tx0, ty0, _ := world.TileFromScreenWithCam(0, 0, camX, camY)
	tx1, ty1, _ := world.TileFromScreenWithCam(ww-1, wh-1, camX, camY)
	if tx0 > tx1 {
		tx0, tx1 = tx1, tx0
	}
	if ty0 > ty1 {
		ty0, ty1 = ty1, ty0
	}
	return tx0 - margin, ty0 - margin, tx1 + margin, ty1 + margin
}
