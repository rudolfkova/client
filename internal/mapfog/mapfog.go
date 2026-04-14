package mapfog

import (
	_ "embed"
	"log"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/rudolfkova/grpc_auth/pkg/gamekit"

	"client/internal/world"
)

//go:embed fog.kage
var fogKage []byte

// Fog процедурный туман по пустым клеткам слоя 0 (см. example/fog/FOG_SHADER.md).
type Fog struct {
	sh    *ebiten.Shader
	start time.Time
}

// NewFog компилирует шейдер; при ошибке sh == nil, Draw не рисует.
func NewFog() *Fog {
	sh, err := ebiten.NewShader(fogKage)
	if err != nil {
		log.Printf("mapfog: shader: %v", err)
		return &Fog{}
	}
	return &Fog{sh: sh, start: time.Now()}
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

// Draw поверх dst: в каждой видимой клетке без тайла на слое 0 — DrawRectShader (как слой 10).
func (f *Fog) Draw(dst *ebiten.Image, tiles []gamekit.Tile, camX, camY float32) {
	if f == nil || f.sh == nil {
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
			var gm ebiten.GeoM
			gm.Translate(float64(rx0), float64(ry0))
			op := &ebiten.DrawRectShaderOptions{
				GeoM: gm,
				Uniforms: map[string]any{
					"Time":     t,
					"EdgeFade": edgeFade,
					"CamX":     camX,
					"CamY":     camY,
				},
			}
			dst.DrawRectShader(tw, th, f.sh, op)
		}
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
