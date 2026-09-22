package lighter

import (
	"context"
	"fmt"

	"github.com/jev-sys/bot/internal/config"
	"github.com/jev-sys/bot/internal/domain"
)

// DualClient routes JEV-aligned orders: leg A = long (buy), leg B = short (sell).
type DualClient struct {
	long  *Client
	short *Client
}

func NewDual(cfg config.Config) (*DualClient, error) {
	long, err := New(cfg.WithLighterLeg(cfg.LighterLegA))
	if err != nil {
		return nil, fmt.Errorf("lighter account A: %w", err)
	}
	short, err := New(cfg.WithLighterLeg(cfg.LighterLegB))
	if err != nil {
		return nil, fmt.Errorf("lighter account B: %w", err)
	}
	return &DualClient{long: long, short: short}, nil
}

func (d *DualClient) GetBook(ctx context.Context, symbol string) (domain.Book, error) {
	return d.long.GetBook(ctx, symbol)
}

func (d *DualClient) GetAccount(ctx context.Context, symbol string) (domain.AccountSnapshot, error) {
	return d.long.GetAccount(ctx, symbol)
}

// GetAccountB returns the short leg (account B) snapshot.
func (d *DualClient) GetAccountB(ctx context.Context, symbol string) (domain.AccountSnapshot, error) {
	return d.short.GetAccount(ctx, symbol)
}

func (d *DualClient) HasPendingBotOrders(ctx context.Context) bool {
	return d.long.HasPendingBotOrders(ctx) || d.short.HasPendingBotOrders(ctx)
}

func (d *DualClient) CancelBotOrders(ctx context.Context, symbol string) error {
	if err := d.long.CancelBotOrders(ctx, symbol); err != nil {
		return err
	}
	return d.short.CancelBotOrders(ctx, symbol)
}

func (d *DualClient) PlaceLimitPostOnly(ctx context.Context, symbol string, tickID int64, intent domain.OrderIntent) (domain.OrderResult, error) {
	if intent.Skip {
		return domain.OrderResult{Status: "skipped"}, nil
	}
	switch intent.Side {
	case domain.SideLong:
		return d.long.PlaceLimitPostOnly(ctx, symbol, tickID, intent)
	case domain.SideShort:
		return d.short.PlaceLimitPostOnly(ctx, symbol, tickID, intent)
	default:
		return domain.OrderResult{Status: "error", Error: "dual: missing order side"}, nil
	}
}
