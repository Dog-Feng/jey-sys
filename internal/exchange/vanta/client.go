package vanta

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jev-sys/bot/internal/book"
	"github.com/jev-sys/bot/internal/config"
	"github.com/jev-sys/bot/internal/domain"
	"github.com/jev-sys/bot/internal/position"
)

// Client implements Exchange for Vanta via Orderly Network REST.
// Docs: https://vanta-6.gitbook.io/vanta-gitbook/core-concepts/api
type Client struct {
	cfg    config.Config
	http   *http.Client
	symbol string
	base   string

	mu            sync.Mutex
	sk            ed25519.PrivateKey
	orderlyKey    string
	accountID     string
	priceTick     float64
	baseTick      float64
	pendingOrder  int64
	clientOrderID string
	nextCOI       int64
}

func New(cfg config.Config) (*Client, error) {
	base := strings.TrimRight(cfg.VantaBaseURL, "/") + "/"
	sym := cfg.VantaSymbol
	if sym == "" {
		sym = "PERP_" + strings.ToUpper(cfg.Symbol) + "_USDC"
	}
	c := &Client{
		cfg:    cfg,
		http:   &http.Client{Timeout: 15 * time.Second},
		symbol: sym,
		base:   base,
		priceTick: 0.1,
		baseTick:  0.00001,
	}
	if err := c.loadSymbolMeta(context.Background()); err != nil {
		return nil, err
	}
	if !cfg.DryRun {
		if cfg.VantaAccountID == "" || cfg.VantaOrderlySecret == "" {
			return nil, fmt.Errorf("vanta live requires VANTA_ORDERLY_ACCOUNT_ID and VANTA_ORDERLY_SECRET")
		}
		sk, pub, err := decodeOrderlySecret(cfg.VantaOrderlySecret)
		if err != nil {
			return nil, err
		}
		c.sk = sk
		c.orderlyKey = strings.TrimSpace(cfg.VantaOrderlyKey)
		if c.orderlyKey == "" {
			c.orderlyKey = pub
		}
		c.accountID = cfg.VantaAccountID
	}
	return c, nil
}

func (c *Client) GetBook(ctx context.Context, _ string) (domain.Book, error) {
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			MidPrice string `json:"mid_price"`
			Spread   string `json:"spread"`
			Asks     []struct {
				Price    string `json:"price"`
				Quantity string `json:"quantity"`
			} `json:"asks"`
			Bids []struct {
				Price    string `json:"price"`
				Quantity string `json:"quantity"`
			} `json:"bids"`
		} `json:"data"`
		Message string `json:"message"`
	}
	body := map[string]any{
		"type":   "orderbook",
		"symbol": c.symbol,
	}
	if err := c.postPublic(ctx, "v1/public/query", body, &resp); err != nil {
		return domain.Book{}, err
	}
	if !resp.Success {
		return domain.Book{}, fmt.Errorf("vanta orderbook: %s", resp.Message)
	}
	if len(resp.Data.Asks) == 0 || len(resp.Data.Bids) == 0 {
		return domain.Book{}, fmt.Errorf("empty vanta book for %s", c.symbol)
	}
	bid, _ := strconv.ParseFloat(resp.Data.Bids[0].Price, 64)
	ask, _ := strconv.ParseFloat(resp.Data.Asks[0].Price, 64)
	bid = book.SnapToTick(bid, c.priceTick)
	ask = book.SnapToTick(ask, c.priceTick)
	mid, _ := strconv.ParseFloat(resp.Data.MidPrice, 64)
	if mid <= 0 {
		mid = (bid + ask) / 2
	} else {
		mid = book.SnapToTick(mid, c.priceTick)
	}
	return domain.Book{
		Symbol:    c.cfg.Symbol,
		BestBid:   bid,
		BestAsk:   ask,
		Mid:       mid,
		SpreadBps: (ask - bid) / mid * 10000,
		TickSize:  c.priceTick,
	}, nil
}

func (c *Client) GetAccount(ctx context.Context, _ string) (domain.AccountSnapshot, error) {
	pos := domain.Position{}
	if c.cfg.DryRun || c.sk == nil {
		return domain.AccountSnapshot{
			Position: pos,
			Allowed:  position.ComputeAllowed(pos, c.cfg.OrderSizeBTC, c.cfg.MaxPositionBTC),
		}, nil
	}
	path := "/v1/position/" + c.symbol
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			PositionQty float64 `json:"position_qty"`
			AverageOpenPrice float64 `json:"average_open_price"`
			UnsettledPnl float64 `json:"unsettled_pnl"`
		} `json:"data"`
		Message string `json:"message"`
	}
	if err := c.getPrivate(ctx, path, &resp); err != nil {
		return domain.AccountSnapshot{}, err
	}
	if resp.Success {
		pos.SizeBTC = resp.Data.PositionQty
		pos.EntryPrice = resp.Data.AverageOpenPrice
		pos.Unrealized = resp.Data.UnsettledPnl
	}
	allowed := position.ComputeAllowed(pos, c.cfg.OrderSizeBTC, c.cfg.MaxPositionBTC)
	return domain.AccountSnapshot{Position: pos, Allowed: allowed}, nil
}

func (c *Client) HasPendingBotOrders(ctx context.Context) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pendingOrder > 0
}

func (c *Client) CancelBotOrders(ctx context.Context, _ string) error {
	c.mu.Lock()
	oid := c.pendingOrder
	c.mu.Unlock()
	if oid <= 0 || c.sk == nil {
		c.mu.Lock()
		c.pendingOrder = 0
		c.clientOrderID = ""
		c.mu.Unlock()
		return nil
	}
	path := fmt.Sprintf("/v1/order?order_id=%d&symbol=%s", oid, c.symbol)
	var resp orderlyOK
	if err := c.deletePrivate(ctx, path, &resp); err != nil {
		if isBenignCancelErr(err) {
			c.mu.Lock()
			c.pendingOrder = 0
			c.clientOrderID = ""
			c.mu.Unlock()
			return nil
		}
		return err
	}
	c.mu.Lock()
	c.pendingOrder = 0
	c.clientOrderID = ""
	c.mu.Unlock()
	return nil
}

func (c *Client) PlaceLimitPostOnly(ctx context.Context, _ string, tickID int64, intent domain.OrderIntent) (domain.OrderResult, error) {
	if intent.Skip {
		return domain.OrderResult{Status: "skipped"}, nil
	}
	if intent.SizeBTC < 1e-12 {
		return domain.OrderResult{Status: "error", Error: "zero size"}, nil
	}
	c.mu.Lock()
	c.nextCOI++
	coi := c.nextCOI
	c.mu.Unlock()
	clientID := fmt.Sprintf("jev-%d-%d", tickID, coi)
	if len(clientID) > 36 {
		clientID = clientID[:36]
	}

	if c.cfg.DryRun || c.sk == nil {
		return domain.OrderResult{ClientOrderIndex: coi, Status: "sim"}, nil
	}

	side := "BUY"
	if intent.Side == domain.SideShort {
		side = "SELL"
	}
	priceStr := book.FormatOrderlyPrice(intent.Price, c.priceTick)
	qtyStr := book.FormatOrderlyPrice(intent.SizeBTC, c.baseTick)
	qty, _ := strconv.ParseFloat(qtyStr, 64)
	if qty < c.baseTick {
		return domain.OrderResult{Status: "error", Error: "size below base_tick"}, nil
	}
	reqBody := map[string]any{
		"symbol":           c.symbol,
		"order_type":       "POST_ONLY",
		"side":             side,
		"order_price":      json.Number(priceStr),
		"order_quantity":   json.Number(qtyStr),
		"client_order_id":  clientID,
		"reduce_only":      intent.ReduceOnly,
	}
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			OrderID int64 `json:"order_id"`
		} `json:"data"`
		Message string `json:"message"`
	}
	if err := c.postPrivate(ctx, "/v1/order", reqBody, &resp); err != nil {
		return domain.OrderResult{Status: "error", Error: err.Error()}, nil
	}
	if !resp.Success {
		return domain.OrderResult{Status: "error", Error: resp.Message}, nil
	}
	c.mu.Lock()
	c.pendingOrder = resp.Data.OrderID
	c.clientOrderID = clientID
	c.mu.Unlock()
	return domain.OrderResult{ClientOrderIndex: coi, Status: "placed"}, nil
}

type orderlyOK struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (c *Client) loadSymbolMeta(ctx context.Context) error {
	path := "v1/public/info/" + c.symbol
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			QuoteTick float64 `json:"quote_tick"`
			BaseTick  float64 `json:"base_tick"`
		} `json:"data"`
		Message string `json:"message"`
	}
	u := c.base + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	if err := c.do(req, &resp); err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("vanta symbol info: %s", resp.Message)
	}
	if resp.Data.QuoteTick > 0 {
		c.priceTick = resp.Data.QuoteTick
	}
	if resp.Data.BaseTick > 0 {
		c.baseTick = resp.Data.BaseTick
	}
	return nil
}

func isBenignCancelErr(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	for _, sub := range []string{
		"the order is completed",
		"order is completed",
		"already cancelled",
		"already canceled",
		"order not found",
		"\"code\":-1006",
	} {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func (c *Client) postPublic(ctx context.Context, path string, body any, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	u := c.base + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func (c *Client) getPrivate(ctx context.Context, path string, out any) error {
	return c.signedRequest(ctx, http.MethodGet, path, "", out)
}

func (c *Client) postPrivate(ctx context.Context, path string, body any, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return c.signedRequest(ctx, http.MethodPost, path, string(raw), out)
}

func (c *Client) deletePrivate(ctx context.Context, path string, out any) error {
	return c.signedRequest(ctx, http.MethodDelete, path, "", out)
}

func (c *Client) signedRequest(ctx context.Context, method, path, body string, out any) error {
	ts, sig, err := signOrderly(c.sk, method, path, body)
	if err != nil {
		return err
	}
	var r io.Reader = http.NoBody
	if body != "" {
		r = strings.NewReader(body)
	}
	u := c.base + strings.TrimPrefix(path, "/")
	req, err := http.NewRequestWithContext(ctx, method, u, r)
	if err != nil {
		return err
	}
	// https://orderly.network/docs/build-on-omnichain/api-authentication
	switch method {
	case http.MethodGet, http.MethodDelete:
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	default:
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("orderly-account-id", c.accountID)
	req.Header.Set("orderly-key", c.orderlyKey)
	req.Header.Set("orderly-timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("orderly-signature", sig)
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("vanta %s %s: %d %s", req.Method, req.URL.Path, resp.StatusCode, string(b))
	}
	if out != nil {
		if err := json.Unmarshal(b, out); err != nil {
			return err
		}
	}
	return nil
}
