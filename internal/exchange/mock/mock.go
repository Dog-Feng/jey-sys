package mock

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/jev-sys/bot/internal/config"
	"github.com/jev-sys/bot/internal/domain"
	"github.com/jev-sys/bot/internal/position"
)

// Exchange simulates a BTC perp book and inventory for dry-run.
type Exchange struct {
	cfg config.Config
	mu  sync.Mutex

	mid       float64
	tick      float64
	pos       domain.Position
	resting   *restingOrder
	nextCOI   int64
}

type restingOrder struct {
	coi        int64
	side       domain.Side
	price      float64
	size       float64
	reduceOnly bool
	placedAt   time.Time
}

func New(cfg config.Config) *Exchange {
	return &Exchange{
		cfg:  cfg,
		mid:  98500,
		tick: 0.1,
	}
}

func (e *Exchange) GetBook(ctx context.Context, symbol string) (domain.Book, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.driftMid()
	spread := 1.0
	bid := e.mid - spread/2
	ask := e.mid + spread/2
	return domain.Book{
		Symbol:    symbol,
		BestBid:   bid,
		BestAsk:   ask,
		Mid:       e.mid,
		SpreadBps: (ask - bid) / e.mid * 10000,
		Imbalance: 0,
		TickSize:  e.tick,
	}, nil
}

func (e *Exchange) GetAccount(ctx context.Context, symbol string) (domain.AccountSnapshot, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.trySimFillLocked()
	allowed := position.ComputeAllowed(e.pos, e.cfg.OrderSizeBTC, e.cfg.MaxPositionBTC)
	return domain.AccountSnapshot{Position: e.pos, Allowed: allowed}, nil
}

func (e *Exchange) CancelBotOrders(ctx context.Context, symbol string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.resting = nil
	return nil
}

func (e *Exchange) PlaceLimitPostOnly(ctx context.Context, symbol string, tickID int64, intent domain.OrderIntent) (domain.OrderResult, error) {
	if intent.Skip {
		return domain.OrderResult{Status: "skipped"}, nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.nextCOI++
	coi := e.nextCOI
	e.resting = &restingOrder{
		coi: coi, side: intent.Side, price: intent.Price,
		size: intent.SizeBTC, reduceOnly: intent.ReduceOnly, placedAt: time.Now(),
	}
	e.trySimFillLocked()
	return domain.OrderResult{ClientOrderIndex: coi, Status: "sim"}, nil
}

func (e *Exchange) driftMid() {
	// tiny random walk
	e.mid += math.Sin(float64(time.Now().UnixNano())/1e9) * 0.05
}

func (e *Exchange) trySimFillLocked() {
	if e.resting == nil {
		return
	}
	// Simplified: if price crosses mid, fill resting maker.
	filled := false
	switch e.resting.side {
	case domain.SideLong:
		if e.mid <= e.resting.price {
			filled = true
		}
	case domain.SideShort:
		if e.mid >= e.resting.price {
			filled = true
		}
	}
	if !filled {
		return
	}
	delta := e.resting.size
	if e.resting.side == domain.SideShort {
		delta = -delta
	}
	if e.resting.reduceOnly {
		if e.pos.SizeBTC > 0 && delta > 0 {
			delta = min(delta, e.pos.SizeBTC)
		}
		if e.pos.SizeBTC < 0 && delta < 0 {
			delta = max(delta, e.pos.SizeBTC)
		}
	}
	old := e.pos.SizeBTC
	e.pos.SizeBTC += delta
	if math.Abs(e.pos.SizeBTC) < 1e-12 {
		e.pos.SizeBTC = 0
		e.pos.EntryPrice = 0
	}
	if old == 0 || sign(old) == sign(delta) {
		if e.pos.EntryPrice == 0 {
			e.pos.EntryPrice = e.resting.price
		}
	}
	e.resting = nil
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func sign(x float64) int {
	if x > 0 {
		return 1
	}
	if x < 0 {
		return -1
	}
	return 0
}
