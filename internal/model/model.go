package model

import (
	"context"

	"github.com/jev-sys/bot/internal/domain"
)

type Model interface {
	Decide(ctx context.Context, state domain.TradeState) (domain.Decision, error)
}
