package policy

import (
	"testing"

	"github.com/jev-sys/bot/internal/config"
	"github.com/jev-sys/bot/internal/domain"
)

func TestDualBothAtCap(t *testing.T) {
	max := 0.003
	if !DualBothAtCap(domain.Position{SizeBTC: 0.003}, domain.Position{SizeBTC: -0.003}, max, 1e-12) {
		t.Fatal("expected both at cap")
	}
	if DualBothAtCap(domain.Position{SizeBTC: 0.003}, domain.Position{SizeBTC: -0.002}, max, 1e-12) {
		t.Fatal("B not at cap")
	}
}

func TestMapDualUnwind(t *testing.T) {
	cfg := config.Config{OrderSizeBTC: 0.001, QuoteInsideTicks: 1}
	bk := domain.Book{BestBid: 100, BestAsk: 101, TickSize: 0.5}
	u := MapDualUnwind(
		domain.Position{SizeBTC: 0.002},
		domain.Position{SizeBTC: -0.003},
		bk, cfg, 1e-12,
	)
	if u.A.Skip || u.A.Side != domain.SideShort || !u.A.ReduceOnly || u.A.SizeBTC != 0.001 {
		t.Fatalf("A unwind: %+v", u.A)
	}
	if u.B.Skip || u.B.Side != domain.SideLong || !u.B.ReduceOnly || u.B.SizeBTC != 0.001 {
		t.Fatalf("B unwind: %+v", u.B)
	}
}
