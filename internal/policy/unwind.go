package policy

import (
	"github.com/jev-sys/bot/internal/book"
	"github.com/jev-sys/bot/internal/config"
	"github.com/jev-sys/bot/internal/domain"
)

// DualUnwindIntents are reduce-only quotes to flatten A (long leg) and B (short leg).
type DualUnwindIntents struct {
	A domain.OrderIntent
	B domain.OrderIntent
}

// MapDualUnwind builds per-leg reduce-only intents during dual-account UNWIND phase.
// A: short side to reduce long; B: long side to reduce short.
func MapDualUnwind(posA, posB domain.Position, bk domain.Book, cfg config.Config, flatEps float64) DualUnwindIntents {
	size := cfg.OrderSizeBTC
	out := DualUnwindIntents{
		A: domain.OrderIntent{Skip: true, SkipReason: "unwind_a_flat"},
		B: domain.OrderIntent{Skip: true, SkipReason: "unwind_b_flat"},
	}
	if posA.SizeBTC > flatEps {
		sz := min(size, posA.SizeBTC)
		out.A = domain.OrderIntent{
			Side: domain.SideShort, ReduceOnly: true,
			Price: book.QuotePriceShort(bk, cfg.QuoteInsideTicks), SizeBTC: sz,
		}
	}
	if posB.SizeBTC < -flatEps {
		sz := min(size, posB.Abs())
		out.B = domain.OrderIntent{
			Side: domain.SideLong, ReduceOnly: true,
			Price: book.QuotePriceLong(bk, cfg.QuoteInsideTicks), SizeBTC: sz,
		}
	}
	return out
}

// DualBothAtCap is true when A is at max long and B at max short (hard dual threshold).
func DualBothAtCap(posA, posB domain.Position, max, eps float64) bool {
	return posA.SizeBTC >= max-eps && posB.SizeBTC <= -(max-eps)
}

// DualBothFlat is true when both legs have negligible position.
func DualBothFlat(posA, posB domain.Position, flatEps float64) bool {
	return posA.Abs() <= flatEps && posB.Abs() <= flatEps
}
