package book

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// SnapToTick rounds p to the nearest multiple of tick (Orderly / exchange quote_tick).
func SnapToTick(p, tick float64) float64 {
	if tick <= 0 {
		return p
	}
	s := FormatOrderlyPrice(p, tick)
	out, _ := strconv.ParseFloat(s, 64)
	return out
}

// TickDecimalPlaces returns decimal places implied by tick (e.g. 0.1 → 1).
func TickDecimalPlaces(tick float64) int {
	if tick <= 0 {
		return 8
	}
	d := 0
	t := tick
	for d < 12 {
		r := math.Round(t)
		if math.Abs(t-r) < 1e-9 {
			if d == 0 && r >= 1 {
				return 0
			}
			break
		}
		t *= 10
		d++
	}
	return d
}

// FormatOrderlyPrice formats p as a fixed-decimal string aligned to tick (avoids JSON float noise).
func FormatOrderlyPrice(p, tick float64) string {
	if tick <= 0 {
		return strconv.FormatFloat(p, 'f', -1, 64)
	}
	scale := int64(math.Round(1.0 / tick))
	if scale <= 0 {
		return strconv.FormatFloat(p, 'f', -1, 64)
	}
	n := int64(math.Round(p / tick))
	dec := TickDecimalPlaces(tick)
	neg := n < 0
	if neg {
		n = -n
	}
	ip := n / scale
	fp := n % scale
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	b.WriteString(strconv.FormatInt(ip, 10))
	if dec > 0 {
		b.WriteByte('.')
		b.WriteString(fmt.Sprintf("%0*d", dec, fp))
	}
	return b.String()
}
