package book

import "github.com/jev-sys/bot/internal/domain"

// QuotePriceLong returns a non-crossing bid-side price (inside touch).
func QuotePriceLong(b domain.Book, insideTicks int) float64 {
	tick := b.TickSize
	if tick <= 0 {
		tick = 0.1
	}
	step := float64(insideTicks) * tick
	p := b.BestBid + step
	if p >= b.BestAsk {
		p = b.BestBid
	}
	return roundToTick(p, tick)
}

// QuotePriceShort returns a non-crossing ask-side price (inside touch).
func QuotePriceShort(b domain.Book, insideTicks int) float64 {
	tick := b.TickSize
	if tick <= 0 {
		tick = 0.1
	}
	step := float64(insideTicks) * tick
	p := b.BestAsk - step
	if p <= b.BestBid {
		p = b.BestAsk
	}
	return roundToTick(p, tick)
}

func roundToTick(p, tick float64) float64 {
	if tick <= 0 {
		return p
	}
	return float64(int64(p/tick+1e-9)) * tick
}

func SpreadBps(bid, ask, mid float64) float64 {
	if mid <= 0 {
		return 0
	}
	return (ask - bid) / mid * 10000
}
