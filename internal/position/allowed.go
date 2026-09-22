package position

import (
	"github.com/jev-sys/bot/internal/domain"
)

func ComputeAllowed(pos domain.Position, orderSize, maxPos float64) domain.Allowed {
	size := pos.SizeBTC
	a := domain.Allowed{
		IncreaseLong:  projectedLong(size, orderSize) <= maxPos+1e-12,
		IncreaseShort: projectedShort(size, orderSize) <= maxPos+1e-12,
		ReduceLong:    size > 1e-12,
		ReduceShort:   size < -1e-12,
	}
	return a
}

func projectedLong(size, orderSize float64) float64 {
	if size < 0 {
		// reducing short first — not increasing long exposure
		return max(0, size+orderSize)
	}
	return size + orderSize
}

func projectedShort(size, orderSize float64) float64 {
	if size > 0 {
		return max(0, size-orderSize)
	}
	return abs(size) + orderSize
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
