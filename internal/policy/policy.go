package policy

import (
	"github.com/jev-sys/bot/internal/book"
	"github.com/jev-sys/bot/internal/config"
	"github.com/jev-sys/bot/internal/domain"
)

// Map turns a JEV buy/sell into a single post-only order intent (Style A).
func Map(dec domain.Decision, pos domain.Position, allowed domain.Allowed, bk domain.Book, cfg config.Config) domain.OrderIntent {
	if dec.Action == domain.ActionHold || dec.Late {
		return domain.OrderIntent{Skip: true, SkipReason: "hold_or_late"}
	}
	wantBuy := dec.Action == domain.ActionBuy
	size := cfg.OrderSizeBTC

	if wantBuy {
		if pos.SizeBTC < -1e-12 {
			sz := min(size, pos.Abs())
			return domain.OrderIntent{
				Side: domain.SideLong, ReduceOnly: true,
				Price: book.QuotePriceLong(bk, cfg.QuoteInsideTicks), SizeBTC: sz,
			}
		}
		if pos.SizeBTC >= cfg.MaxPositionBTC-1e-12 {
			return domain.OrderIntent{Skip: true, Capped: true, SkipReason: "max_long"}
		}
		if !allowed.IncreaseLong {
			return domain.OrderIntent{Skip: true, SkipReason: "not_allowed_long"}
		}
		return domain.OrderIntent{
			Side: domain.SideLong, ReduceOnly: false,
			Price: book.QuotePriceLong(bk, cfg.QuoteInsideTicks), SizeBTC: size,
		}
	}

	// sell → short side
	if pos.SizeBTC > 1e-12 {
		sz := min(size, pos.Abs())
		return domain.OrderIntent{
			Side: domain.SideShort, ReduceOnly: true,
			Price: book.QuotePriceShort(bk, cfg.QuoteInsideTicks), SizeBTC: sz,
		}
	}
	if pos.Abs() >= cfg.MaxPositionBTC-1e-12 && pos.SizeBTC < 0 {
		return domain.OrderIntent{Skip: true, Capped: true, SkipReason: "max_short"}
	}
	if !allowed.IncreaseShort {
		return domain.OrderIntent{Skip: true, SkipReason: "not_allowed_short"}
	}
	return domain.OrderIntent{
		Side: domain.SideShort, ReduceOnly: false,
		Price: book.QuotePriceShort(bk, cfg.QuoteInsideTicks), SizeBTC: size,
	}
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
