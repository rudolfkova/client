package tiles

// NormalizeRotationQuarter приводит r к 0..3 (четверти по часовой, как gamekit.NormalizeTileRotationQuarter).
func NormalizeRotationQuarter(r int) int {
	return ((r % 4) + 4) % 4
}
