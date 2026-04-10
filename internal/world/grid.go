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
	cx = float32(GridPad) + float32(tileX)*TileSize + TileSize*0.5
	cy = float32(GridPad) + float32(tileY)*TileSize + TileSize*0.5
	return cx, cy
}

// TileFromScreen возвращает индекс клетки под курсором (в т.ч. отрицательные x/y), согласованно с tiles.Draw и ToScreen.
func TileFromScreen(mx, my int) (tx, ty int, ok bool) {
	return TileFromScreenWithCam(mx, my, 0, 0)
}

// TileFromScreenWithCam — то же с учётом смещения камеры (мир в px: +cam = сдвиг взгляда вправо/вниз).
func TileFromScreenWithCam(mx, my int, camX, camY float32) (tx, ty int, ok bool) {
	fx := float32(mx) + camX
	fy := float32(my) + camY
	// Floor, как для положительных координат, так и для отрицательных (int отрицательной дроби в Go — к нулю, не к −∞).
	tx = int(math.Floor(float64(fx-float32(GridPad)) / float64(TileSize)))
	ty = int(math.Floor(float64(fy-float32(GridPad)) / float64(TileSize)))
	return tx, ty, true
}
