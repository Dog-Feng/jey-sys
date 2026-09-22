package exchange

import (
	"context"

	"github.com/jev-sys/bot/internal/domain"
)

// Exchange abstracts Lighter (live or simulated).
type Exchange interface {
	GetBook(ctx context.Context, symbol string) (domain.Book, error)
	GetAccount(ctx context.Context, symbol string) (domain.AccountSnapshot, error)
	CancelBotOrders(ctx context.Context, symbol string) error
	PlaceLimitPostOnly(ctx context.Context, symbol string, tickID int64, intent domain.OrderIntent) (domain.OrderResult, error)
}
