package signer

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	lighterclient "github.com/elliottech/lighter-go/client"
	lighterhttp "github.com/elliottech/lighter-go/client/http"
	"github.com/elliottech/lighter-go/types"
	"github.com/elliottech/lighter-go/types/txtypes"

	"github.com/jev-sys/bot/internal/config"
)

var sendTxHTTP = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		MaxIdleConnsPerHost: 8,
		IdleConnTimeout:     90 * time.Second,
	},
}

// Signer signs L2 txs with lighter-go and submits them via REST sendTx.
type Signer struct {
	cfg    config.Config
	http   *http.Client
	tx *lighterclient.TxClient
}

type CreateOrderParams struct {
	MarketIndex      int16
	ClientOrderIndex int64
	BaseAmount       int64
	Price            uint32
	IsAsk            uint8
	ReduceOnly       uint8
	OrderExpiry      int64
}

func New(cfg config.Config) (*Signer, error) {
	pk := strings.TrimSpace(cfg.APIPrivateKey)
	if pk == "" {
		return nil, fmt.Errorf("empty LIGHTER_API_PRIVATE_KEY")
	}
	if cfg.AccountIndex <= 0 {
		return nil, fmt.Errorf("invalid LIGHTER_ACCOUNT_INDEX")
	}
	host := normalizeHost(cfg.LighterHost)
	api := lighterhttp.NewClient(host)
	if api == nil {
		return nil, fmt.Errorf("invalid LIGHTER_HOST")
	}
	txClient, err := lighterclient.CreateClient(api, pk, cfg.LighterChainID, cfg.APIKeyIndex, cfg.AccountIndex)
	if err != nil {
		return nil, fmt.Errorf("CreateClient: %w", err)
	}
	if err := checkWithRetry(txClient, 3); err != nil {
		return nil, fmt.Errorf("lighter Check (api key vs account): %w", err)
	}
	return &Signer{
		cfg:  cfg,
		http: sendTxHTTP,
		tx:   txClient,
	}, nil
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	return strings.TrimRight(host, "/")
}

func checkWithRetry(tx *lighterclient.TxClient, attempts int) error {
	if attempts < 1 {
		attempts = 1
	}
	var last error
	for i := 0; i < attempts; i++ {
		if err := tx.Check(); err != nil {
			last = err
			if i+1 < attempts && isRetryableLighterNetErr(err) {
				time.Sleep(time.Duration(i+1) * 2 * time.Second)
				continue
			}
			return err
		}
		return nil
	}
	return last
}

func isRetryableLighterNetErr(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "connection reset")
}

func (s *Signer) CreateLimitPostOnly(ctx context.Context, p CreateOrderParams) error {
	expiry := p.OrderExpiry
	if expiry <= 0 {
		expiry = time.Now().Add(10 * time.Minute).UnixMilli()
	}
	req := &types.CreateOrderTxReq{
		MarketIndex:      p.MarketIndex,
		ClientOrderIndex: p.ClientOrderIndex,
		BaseAmount:       p.BaseAmount,
		Price:            p.Price,
		IsAsk:            p.IsAsk,
		Type:             txtypes.LimitOrder,
		TimeInForce:      txtypes.PostOnly,
		ReduceOnly:       p.ReduceOnly,
		TriggerPrice:     txtypes.NilOrderTriggerPrice,
		OrderExpiry:      expiry,
	}
	txInfo, err := s.tx.GetCreateOrderTransaction(req, nil)
	if err != nil {
		return err
	}
	_, err = postSendTx(ctx, s.http, s.cfg.LighterHost, txInfo)
	return err
}

func (s *Signer) CancelOrder(ctx context.Context, marketIndex int16, clientOrderIndex int64) error {
	req := &types.CancelOrderTxReq{
		MarketIndex: marketIndex,
		Index:       clientOrderIndex,
	}
	txInfo, err := s.tx.GetCancelOrderTransaction(req, nil)
	if err != nil {
		return err
	}
	_, err = postSendTx(ctx, s.http, s.cfg.LighterHost, txInfo)
	return err
}

// UpdateLeverage sets cross/isolated leverage once at startup (optional).
func (s *Signer) UpdateLeverage(ctx context.Context, marketIndex int16, leverage int, cross bool) error {
	if leverage <= 0 {
		return nil
	}
	// initial_margin_fraction: 10000 / leverage (basis on MarginFractionTick)
	imf := uint16(10000 / leverage)
	marginMode := txtypes.CrossMargin
	if !cross {
		marginMode = txtypes.IsolatedMargin
	}
	req := &types.UpdateLeverageTxReq{
		MarketIndex:           marketIndex,
		InitialMarginFraction: imf,
		MarginMode:            uint8(marginMode),
	}
	txInfo, err := s.tx.GetUpdateLeverageTransaction(req, nil)
	if err != nil {
		return err
	}
	_, err = postSendTx(ctx, s.http, s.cfg.LighterHost, txInfo)
	return err
}
