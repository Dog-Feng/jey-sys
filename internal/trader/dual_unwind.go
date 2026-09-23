package trader

import (
	"context"
	"time"

	"github.com/jev-sys/bot/internal/domain"
	"github.com/jev-sys/bot/internal/policy"
)

func (t *Trader) tryEnterUnwind(posA, posB domain.Position) {
	if policy.DualBothAtCap(posA, posB, t.cfg.MaxPositionBTC, t.cfg.UnwindFlatEps) {
		t.dualCapStreak++
	} else {
		t.dualCapStreak = 0
		return
	}
	if t.dualCapStreak < t.cfg.UnwindEnterConfirmTicks {
		return
	}
	t.dualPhase = domain.DualPhaseUnwind
	t.dualCapStreak = 0
	t.dualFlatStreak = 0
	t.unwindTicks = 0
	t.log.Info("unwind_start",
		"pos_a", posA.SizeBTC,
		"pos_b", posB.SizeBTC,
		"max", t.cfg.MaxPositionBTC,
	)
}

func (t *Trader) tryExitUnwind(posA, posB domain.Position) {
	if t.dualPhase != domain.DualPhaseUnwind {
		return
	}
	if !policy.DualBothFlat(posA, posB, t.cfg.UnwindFlatEps) {
		t.dualFlatStreak = 0
		return
	}
	t.dualFlatStreak++
	if t.dualFlatStreak < t.cfg.UnwindExitConfirmTicks {
		return
	}
	t.dualPhase = domain.DualPhaseNormal
	t.dualFlatStreak = 0
	t.log.Info("unwind_complete", "pos_a", posA.SizeBTC, "pos_b", posB.SizeBTC)
}

func (t *Trader) runUnwindTick(
	ctx context.Context,
	start, afterBook, afterJev time.Time,
	bk domain.Book,
	acctA, acctB domain.AccountSnapshot,
	dec domain.Decision,
) error {
	t.unwindTicks++
	intents := policy.MapDualUnwind(acctA.Position, acctB.Position, bk, t.cfg, t.cfg.UnwindFlatEps)

	needExec := !intents.A.Skip || !intents.B.Skip || t.ex.HasPendingBotOrders(ctx)
	if needExec {
		if err := t.ex.CancelBotOrders(ctx, t.cfg.Symbol); err != nil {
			return err
		}
	}

	var orderA, orderB domain.OrderResult
	var err error
	if !intents.A.Skip {
		orderA, err = t.ex.PlaceLimitPostOnly(ctx, t.cfg.Symbol, t.tickID, intents.A)
		if err != nil {
			return err
		}
	} else {
		orderA = domain.OrderResult{Status: "skipped"}
	}
	if !intents.B.Skip {
		orderB, err = t.ex.PlaceLimitPostOnly(ctx, t.cfg.Symbol, t.tickID, intents.B)
		if err != nil {
			return err
		}
	} else {
		orderB = domain.OrderResult{Status: "skipped"}
	}
	afterExec := time.Now()

	if orderA.Status == "placed" || orderB.Status == "placed" {
		if freshA, err := t.ex.GetAccount(ctx, t.cfg.Symbol); err == nil {
			acctA = freshA
		}
		if freshB, err := t.lighterAccountB(ctx); err == nil {
			acctB = freshB
		}
	}
	t.tryExitUnwind(acctA.Position, acctB.Position)

	ev := domain.TickEvent{
		TickID:    t.tickID,
		TsMs:      time.Now().UnixMilli(),
		Symbol:    t.cfg.Symbol,
		Mid:       bk.Mid,
		BestBid:   bk.BestBid,
		BestAsk:   bk.BestAsk,
		SpreadBps: bk.SpreadBps,
		Decision:  &dec,
		Intent:    intents.A,
		Order:     orderA,
		Position:  acctA.Position,
		Phase:     string(t.dualPhase),
	}
	t.lastEvent = &ev
	_ = t.store.Append(ev)

	t.log.Info("tick",
		"id", ev.TickID,
		"phase", t.dualPhase,
		"unwind_ticks", t.unwindTicks,
		"action", dec.Action,
		"intent_a_side", intents.A.Side,
		"intent_a_skip", intents.A.Skip,
		"intent_a_reason", intents.A.SkipReason,
		"order_a", orderA.Status,
		"order_a_err", orderA.Error,
		"intent_b_side", intents.B.Side,
		"intent_b_skip", intents.B.Skip,
		"intent_b_reason", intents.B.SkipReason,
		"order_b", orderB.Status,
		"order_b_err", orderB.Error,
		"pos_a", acctA.Position.SizeBTC,
		"pos_b", acctB.Position.SizeBTC,
		"exec_leg", "unwind_both",
		"ms", time.Since(start).Milliseconds(),
		"ms_book", afterBook.Sub(start).Milliseconds(),
		"ms_jev", afterJev.Sub(afterBook).Milliseconds(),
		"ms_exec", afterExec.Sub(afterJev).Milliseconds(),
	)
	return nil
}
