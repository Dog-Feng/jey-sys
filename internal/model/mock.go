package model

import (
	"context"
	"time"

	"github.com/jev-sys/bot/internal/domain"
)

// MockModel uses short-horizon momentum on returnsBps.last5.
type MockModel struct{}

func NewMock() *MockModel { return &MockModel{} }

func (m *MockModel) Decide(ctx context.Context, state domain.TradeState) (domain.Decision, error) {
	start := time.Now()
	ret := state.ReturnsBps["last5"]
	action := domain.ActionBuy
	pBuy := 0.55
	if ret < 0 {
		action = domain.ActionSell
		pBuy = 0.45
	}
	return domain.Decision{
		Action: action,
		Probabilities: map[domain.Action]float64{
			domain.ActionBuy:  pBuy,
			domain.ActionSell: 1 - pBuy,
		},
		Confidence: max(pBuy, 1-pBuy),
		LatencyMs:  time.Since(start).Milliseconds(),
	}, nil
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
