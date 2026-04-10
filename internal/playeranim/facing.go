package playeranim

import "github.com/rudolfkova/grpc_auth/pkg/gamekit"

// Оси как в game-service / gamekit: +FaceDX — сторона роста координаты клетки x («вправо»), +FaceDY — y («вниз»).

// Индексы кардинального направления для ряда спрайта (E → S → W → N по часовой на экране).
const (
	CardinalE = 0
	CardinalS = 1
	CardinalW = 2
	CardinalN = 3
)

// ClampAxis приводит значение к {-1, 0, 1} (на случай старых или битых пакетов).
func ClampAxis(v int) int {
	if v < -1 {
		return -1
	}
	if v > 1 {
		return 1
	}
	return v
}

// CardinalFromFace маппит face_dx / face_dy в 0..3. При обоих ненулевых приоритет у X (полушаг «лесенки»).
func CardinalFromFace(dx, dy int) int {
	dx, dy = ClampAxis(dx), ClampAxis(dy)
	if dx != 0 && dy != 0 {
		dy = 0
	}
	switch {
	case dx > 0:
		return CardinalE
	case dx < 0:
		return CardinalW
	case dy > 0:
		return CardinalS
	case dy < 0:
		return CardinalN
	default:
		return CardinalE
	}
}

// CardinalFromPlayer — то же из снимка state.
func CardinalFromPlayer(p gamekit.Player) int {
	return CardinalFromFace(p.FaceDX, p.FaceDY)
}
