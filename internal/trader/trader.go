package trader

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/jev-sys/bot/internal/config"
	"github.com/jev-sys/bot/internal/domain"
	"github.com/jev-sys/bot/internal/exchange"
	"github.com/jev-sys/bot/internal/model"
	"github.com/jev-sys/bot/internal/policy"
	"github.com/jev-sys/bot/internal/store"
)

type Trader struct {
	cfg   config.Config
	ex    exchange.Exchange
	model model.Model
	store *store.JSONL
	log   *slog.Logger

	mu        sync.Mutex
	tickID    int64
	mids      []float64
	lastEvent *domain.TickEvent
}

func New(cfg config.Config, ex exchange.Exchange, m model.Model, st *store.JSONL, logger *slog.Logger) *Trader {
	return &Trader{cfg: cfg, ex: ex, model: m, store: st, log: logger}
}

func (t *Trader) LastEvent() *domain.TickEvent {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastEvent
}

func (t *Trader) Run(ctx context.Context) error {
	ticker := time.NewTicker(t.cfg.TickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := t.onTick(ctx); err != nil {
				t.log.Error("tick", "err", err)
			}
		}
	}
}

func (t *Trader) onTick(ctx context.Context) error {
	if !t.mu.TryLock() {
		t.emitLate()
		return nil
	}
	defer t.mu.Unlock()

	t.tickID++
	start := time.Now()

	var bk domain.Book
	var acct domain.AccountSnapshot
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		bk, err = t.ex.GetBook(gctx, t.cfg.Symbol)
		return err
	})
	g.Go(func() error {
		var err error
		acct, err = t.ex.GetAccount(gctx, t.cfg.Symbol)
		return err
	})
	if err := g.Wait(); err != nil {
		return err
	}
	afterBook := time.Now()

	t.appendMid(bk.Mid)
	state := t.buildState(bk, acct)

	dec, err := t.model.Decide(ctx, state)
	if err != nil {
		return err
	}
	afterJev := time.Now()
	if t.cfg.JevMinConfidence > 0 && dec.Confidence < t.cfg.JevMinConfidence {
		if dec.Action == domain.ActionBuy && acct.Position.SizeBTC <= 0 {
			dec.Action = domain.ActionHold
		}
		if dec.Action == domain.ActionSell && acct.Position.SizeBTC >= 0 {
			dec.Action = domain.ActionHold
		}
	}
	if t.cfg.MinSpreadBps > 0 && bk.SpreadBps > t.cfg.MinSpreadBps {
		dec.Action = domain.ActionHold
	}

	intent := policy.Map(dec, acct.Position, acct.Allowed, bk, t.cfg)
	if !intent.Skip || t.ex.HasPendingBotOrders(ctx) {
		if err := t.ex.CancelBotOrders(ctx, t.cfg.Symbol); err != nil {
			return err
		}
	}
	order, err := t.ex.PlaceLimitPostOnly(ctx, t.cfg.Symbol, t.tickID, intent)
	if err != nil {
		return err
	}
	afterExec := time.Now()

	posSnap := acct
	if !intent.Skip && order.Status == "placed" {
		if fresh, err := t.ex.GetAccount(ctx, t.cfg.Symbol); err == nil {
			posSnap = fresh
		}
	}
	ev := domain.TickEvent{
		TickID:    t.tickID,
		TsMs:      time.Now().UnixMilli(),
		Symbol:    t.cfg.Symbol,
		Mid:       bk.Mid,
		BestBid:   bk.BestBid,
		BestAsk:   bk.BestAsk,
		SpreadBps: bk.SpreadBps,
		Decision:  &dec,
		Intent:    intent,
		Order:     order,
		Position:  posSnap.Position,
	}
	t.lastEvent = &ev

	_ = t.store.Append(ev)
	t.log.Info("tick",
		"id", ev.TickID,
		"action", dec.Action,
		"intent_side", intent.Side,
		"reduce_only", intent.ReduceOnly,
		"skip", intent.Skip,
		"order", order.Status,
		"order_err", order.Error,
		"pos", posSnap.Position.SizeBTC,
		"ms", time.Since(start).Milliseconds(),
		"ms_book", afterBook.Sub(start).Milliseconds(),
		"ms_jev", afterJev.Sub(afterBook).Milliseconds(),
		"ms_exec", afterExec.Sub(afterJev).Milliseconds(),
	)
	return nil
}

func (t *Trader) emitLate() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tickID++
	ev := domain.TickEvent{
		TickID: t.tickID,
		TsMs:   time.Now().UnixMilli(),
		Symbol: t.cfg.Symbol,
		Late:   true,
		Decision: &domain.Decision{
			Action:        domain.ActionHold,
			Late:          true,
			Probabilities: map[domain.Action]float64{domain.ActionHold: 1},
		},
	}
	t.lastEvent = &ev
	_ = t.store.Append(ev)
	t.log.Warn("late tick", "id", ev.TickID)
}

func (t *Trader) appendMid(mid float64) {
	t.mids = append(t.mids, mid)
	if len(t.mids) > 400 {
		t.mids = t.mids[len(t.mids)-400:]
	}
}

func (t *Trader) buildState(bk domain.Book, acct domain.AccountSnapshot) domain.TradeState {
	ret := func(k int) float64 {
		n := len(t.mids)
		if n <= k {
			return 0
		}
		old := t.mids[n-1-k]
		cur := t.mids[n-1]
		if old == 0 {
			return 0
		}
		return (cur - old) / old * 10000
	}
	sample := make([]string, 0, 20)
	for i := len(t.mids) - 1; i >= 0 && len(sample) < 20; i -= 5 {
		sample = append(sample, strconv.FormatFloat(t.mids[i], 'f', 2, 64))
	}
	recent := ""
	for i, s := range sample {
		if i > 0 {
			recent += " "
		}
		recent += s
	}
	mid := bk.Mid
	if len(t.mids) > 0 {
		mid = t.mids[len(t.mids)-1]
	}
	return domain.TradeState{
		Symbol:       t.cfg.Symbol,
		TickID:       t.tickID,
		HorizonTicks: t.cfg.HorizonTicks,
		TickInterval: t.cfg.TickInterval.String(),
		Mid:          mid,
		SpreadBps:    bk.SpreadBps,
		ReturnsBps: map[string]float64{
			"last1":  ret(1),
			"last5":  ret(5),
			"last20": ret(20),
		},
		RecentMids: recent,
		Position:   acct.Position,
		Allowed:    acct.Allowed,
	}
}
