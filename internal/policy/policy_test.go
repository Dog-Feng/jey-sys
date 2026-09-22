package policy

import (
	"testing"

	"github.com/jev-sys/bot/internal/config"
	"github.com/jev-sys/bot/internal/domain"
)

func TestMapReduceShort(t *testing.T) {
	cfg := config.Config{OrderSizeBTC: 0.001, MaxPositionBTC: 0.003, QuoteInsideTicks: 1}
	bk := domain.Book{BestBid: 100, BestAsk: 101, Mid: 100.5, TickSize: 0.5}
	dec := domain.Decision{Action: domain.ActionBuy}
	pos := domain.Position{SizeBTC: -0.002}
	allowed := domain.Allowed{IncreaseLong: true, ReduceShort: true}

	intent := Map(dec, pos, allowed, bk, cfg)
	if !intent.ReduceOnly || intent.Side != domain.SideLong {
		t.Fatalf("expected reduce long, got %+v", intent)
	}
	if intent.SizeBTC != 0.001 {
		t.Fatalf("size want 0.001 got %v", intent.SizeBTC)
	}
}

func TestMapMaxLongSkip(t *testing.T) {
	cfg := config.Config{OrderSizeBTC: 0.001, MaxPositionBTC: 0.003, QuoteInsideTicks: 1}
	bk := domain.Book{BestBid: 100, BestAsk: 101, TickSize: 0.5}
	dec := domain.Decision{Action: domain.ActionBuy}
	pos := domain.Position{SizeBTC: 0.003}
	intent := Map(dec, pos, domain.Allowed{}, bk, cfg)
	if !intent.Skip || !intent.Capped {
		t.Fatalf("expected capped skip, got %+v", intent)
	}
}
