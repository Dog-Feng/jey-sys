package policy

import (
	"testing"

	"github.com/jev-sys/bot/internal/config"
	"github.com/jev-sys/bot/internal/domain"
)

func TestMapJevBuyQuotesLongFlat(t *testing.T) {
	cfg := config.Config{OrderSizeBTC: 0.001, MaxPositionBTC: 0.003, QuoteInsideTicks: 1}
	bk := domain.Book{BestBid: 100, BestAsk: 101, Mid: 100.5, TickSize: 0.5}
	dec := domain.Decision{Action: domain.ActionBuy}
	intent := Map(dec, domain.Position{}, domain.Allowed{IncreaseLong: true}, bk, cfg)
	if intent.Skip || intent.Side != domain.SideLong || intent.ReduceOnly {
		t.Fatalf("expected open long quote, got %+v", intent)
	}
}

func TestMapJevSellQuotesShortFlat(t *testing.T) {
	cfg := config.Config{OrderSizeBTC: 0.001, MaxPositionBTC: 0.003, QuoteInsideTicks: 1}
	bk := domain.Book{BestBid: 100, BestAsk: 101, Mid: 100.5, TickSize: 0.5}
	dec := domain.Decision{Action: domain.ActionSell}
	intent := Map(dec, domain.Position{}, domain.Allowed{IncreaseShort: true}, bk, cfg)
	if intent.Skip || intent.Side != domain.SideShort || intent.ReduceOnly {
		t.Fatalf("expected open short quote, got %+v", intent)
	}
}

func TestMapReduceShortViaBuy(t *testing.T) {
	cfg := config.Config{OrderSizeBTC: 0.001, MaxPositionBTC: 0.003, QuoteInsideTicks: 1}
	bk := domain.Book{BestBid: 100, BestAsk: 101, Mid: 100.5, TickSize: 0.5}
	dec := domain.Decision{Action: domain.ActionBuy}
	pos := domain.Position{SizeBTC: -0.002}
	allowed := domain.Allowed{IncreaseLong: true, ReduceShort: true}

	intent := Map(dec, pos, allowed, bk, cfg)
	if !intent.ReduceOnly || intent.Side != domain.SideLong {
		t.Fatalf("expected reduce short via long quote, got %+v", intent)
	}
	if intent.SizeBTC != 0.001 {
		t.Fatalf("size want 0.001 got %v", intent.SizeBTC)
	}
}

func TestMapReduceLongViaSell(t *testing.T) {
	cfg := config.Config{OrderSizeBTC: 0.001, MaxPositionBTC: 0.003, QuoteInsideTicks: 1}
	bk := domain.Book{BestBid: 100, BestAsk: 101, Mid: 100.5, TickSize: 0.5}
	dec := domain.Decision{Action: domain.ActionSell}
	pos := domain.Position{SizeBTC: 0.002}
	intent := Map(dec, pos, domain.Allowed{IncreaseShort: true}, bk, cfg)
	if !intent.ReduceOnly || intent.Side != domain.SideShort {
		t.Fatalf("expected reduce long via short quote, got %+v", intent)
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

func TestMapMaxShortSkip(t *testing.T) {
	cfg := config.Config{OrderSizeBTC: 0.001, MaxPositionBTC: 0.003, QuoteInsideTicks: 1}
	bk := domain.Book{BestBid: 100, BestAsk: 101, TickSize: 0.5}
	dec := domain.Decision{Action: domain.ActionSell}
	pos := domain.Position{SizeBTC: -0.003}
	intent := Map(dec, pos, domain.Allowed{}, bk, cfg)
	if !intent.Skip || !intent.Capped {
		t.Fatalf("expected capped skip, got %+v", intent)
	}
}
