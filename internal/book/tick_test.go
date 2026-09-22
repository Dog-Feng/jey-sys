package book

import (
	"testing"

	"github.com/jev-sys/bot/internal/domain"
)

func TestFormatOrderlyPrice(t *testing.T) {
	tick := 0.1
	cases := []struct {
		in  float64
		out string
	}{
		{85391.2, "85391.2"},
		{85391.19999999999, "85391.2"},
		{85391.05, "85391.1"},
	}
	for _, c := range cases {
		got := FormatOrderlyPrice(c.in, tick)
		if got != c.out {
			t.Fatalf("FormatOrderlyPrice(%v) = %q want %q", c.in, got, c.out)
		}
	}
}

func TestQuotePriceFloatNoise(t *testing.T) {
	bk := domain.Book{BestBid: 85390.4, BestAsk: 85391.2, TickSize: 0.1}
	p := QuotePriceShort(bk, 1)
	got := FormatOrderlyPrice(p, 0.1)
	if got != "85391.1" {
		t.Fatalf("short quote %q want 85391.1", got)
	}
}
