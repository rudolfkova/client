package world

import "math"

const (
	TileSize      = 48
	GridPad       = 40.0
	PlayerRadius  = 18.0
	LabelTextSize = 16.0
	LabelAboveGap = 6.0
)

func ToScreen(tileX, tileY int) (cx, cy float32) {
	return ToScreenCenterF(float32(tileX), float32(tileY))
}

// ToScreenCenterF центр «клетки» в дробных координатах сетки (для плавного движения спрайта).
func ToScreenCenterF(tileX, tileY float32) (cx, cy float32) {
	ts := float32(TileSize)
	cx = float32(GridPad) + tileX*ts + ts*0.5
	cy = float32(GridPad) + tileY*ts + ts*0.5
	return cx, cy
}

// TileFromScreen возвращает индекс клетки под курсором (в т.ч. отрицательные x/y), согласованно с tiles.Draw и ToScreen.
func TileFromScreen(mx, my int) (tx, ty int, ok bool) {
	return TileFromScreenWithCam(mx, my, 0, 0)
}

// TileFromScreenWithCam — то же с учётом смещения камеры (мир в px: +cam = сдвиг взгляда вправо/вниз).
func TileFromScreenWithCam(mx, my int, camX, camY float32) (tx, ty int, ok bool) {
	return TileFromScreenWithCamZoom(mx, my, camX, camY, 1)
}

// TileFromScreenWithCamZoom — как TileFromScreenWithCam, но экранные координаты делятся на zoom
// (тайлы рисуются как (мир − cam) * zoom). При zoom ≤ 0 используется 1.
func TileFromScreenWithCamZoom(mx, my int, camX, camY, zoom float32) (tx, ty int, ok bool) {
	z := zoom
	if z <= 0 {
		z = 1
	}
	fx := float32(mx)/z + camX
	fy := float32(my)/z + camY
	tx = int(math.Floor(float64(fx-float32(GridPad)) / float64(TileSize)))
	ty = int(math.Floor(float64(fy-float32(GridPad)) / float64(TileSize)))
	return tx, ty, true
}
