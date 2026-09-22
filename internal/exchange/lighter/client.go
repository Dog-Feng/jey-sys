package lighter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jev-sys/bot/internal/config"
	"github.com/jev-sys/bot/internal/domain"
	"github.com/jev-sys/bot/internal/exchange/lighter/signer"
	"github.com/jev-sys/bot/internal/position"
)

var lighterHTTP = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		MaxIdleConnsPerHost: 16,
		IdleConnTimeout:     90 * time.Second,
	},
}

// Client talks to RB Lighter REST + signed L2 txs.
type Client struct {
	cfg    config.Config
	http   *http.Client
	signer *signer.Signer

	mu          sync.Mutex
	marketIndex int16
	sizeDecimals int
	priceDecimals int
	minBase      float64
	tickSize     float64
	coiSet       map[int64]struct{}
	nextCOI      int64
}

func New(cfg config.Config) (*Client, error) {
	if !cfg.DryRun && (cfg.AccountIndex <= 0 || cfg.APIPrivateKey == "") {
		return nil, fmt.Errorf("lighter live requires LIGHTER_ACCOUNT_INDEX and LIGHTER_API_PRIVATE_KEY")
	}
	var s *signer.Signer
	// DRY_RUN: real book only; no L2 signer or apikeys check (see apidocs.rh.lighter.xyz get-started).
	if !cfg.DryRun && cfg.APIPrivateKey != "" {
		var err error
		s, err = signer.New(cfg)
		if err != nil {
			return nil, err
		}
	}
	c := &Client{
		cfg:    cfg,
		http:   lighterHTTP,
		signer: s,
		coiSet: make(map[int64]struct{}),
	}
	if err := c.loadMarketMeta(context.Background(), cfg.Symbol); err != nil {
		return nil, err
	}
	if s != nil && !cfg.DryRun && cfg.LighterLeverage > 0 {
		if err := s.UpdateLeverage(context.Background(), c.marketIndex, cfg.LighterLeverage, true); err != nil {
			return nil, fmt.Errorf("update leverage: %w", err)
		}
	}
	return c, nil
}

func (c *Client) loadMarketMeta(ctx context.Context, symbol string) error {
	var resp struct {
		Code       int `json:"code"`
		OrderBooks []struct {
			Symbol                 string    `json:"symbol"`
			MarketID               int16     `json:"market_id"`
			MarketType             string    `json:"market_type"`
			SupportedSizeDecimals  int       `json:"supported_size_decimals"`
			SupportedPriceDecimals int       `json:"supported_price_decimals"`
			MinBaseAmount          flexFloat `json:"min_base_amount"`
		} `json:"order_books"`
	}
	if err := c.getJSON(ctx, "api/v1/orderBooks", nil, &resp); err != nil {
		return err
	}
	sym := strings.ToUpper(symbol)
	for _, ob := range resp.OrderBooks {
		if !strings.EqualFold(ob.Symbol, sym) {
			continue
		}
		// Prefer perp when resolving bare symbols like BTC.
		if ob.MarketType != "" && ob.MarketType != "perp" && !strings.Contains(sym, "/") {
			continue
		}
		c.marketIndex = ob.MarketID
		c.sizeDecimals = ob.SupportedSizeDecimals
		c.priceDecimals = ob.SupportedPriceDecimals
		c.minBase = float64(ob.MinBaseAmount)
		c.tickSize = math.Pow10(-ob.SupportedPriceDecimals)
		return nil
	}
	return fmt.Errorf("market %s not found on lighter", symbol)
}

func (c *Client) GetBook(ctx context.Context, symbol string) (domain.Book, error) {
	var resp struct {
		Code int `json:"code"`
		Bids []struct {
			Price string `json:"price"`
			Size  string `json:"size"`
		} `json:"bids"`
		Asks []struct {
			Price string `json:"price"`
			Size  string `json:"size"`
		} `json:"asks"`
	}
	params := map[string]string{
		"market_id": strconv.Itoa(int(c.marketIndex)),
		"limit":     "5",
	}
	if err := c.getJSON(ctx, "api/v1/orderBookOrders", params, &resp); err != nil {
		return domain.Book{}, err
	}
	if len(resp.Bids) == 0 || len(resp.Asks) == 0 {
		return domain.Book{}, fmt.Errorf("empty book for %s", symbol)
	}
	bid, _ := strconv.ParseFloat(resp.Bids[0].Price, 64)
	ask, _ := strconv.ParseFloat(resp.Asks[0].Price, 64)
	mid := (bid + ask) / 2
	return domain.Book{
		Symbol:    symbol,
		BestBid:   bid,
		BestAsk:   ask,
		Mid:       mid,
		SpreadBps: (ask - bid) / mid * 10000,
		TickSize:  c.tickSize,
	}, nil
}

func (c *Client) GetAccount(ctx context.Context, symbol string) (domain.AccountSnapshot, error) {
	if c.cfg.AccountIndex <= 0 {
		pos := domain.Position{}
		return domain.AccountSnapshot{
			Position: pos,
			Allowed:  position.ComputeAllowed(pos, c.cfg.OrderSizeBTC, c.cfg.MaxPositionBTC),
		}, nil
	}
	var resp struct {
		Code int `json:"code"`
		Accounts []struct {
			Positions []struct {
				MarketIndex int16  `json:"market_index"`
				MarketID    int16  `json:"market_id"`
				Sign        int     `json:"sign"`
				Position    string  `json:"position"`
				AvgEntryPrice string `json:"avg_entry_price"`
				UnrealizedPnl string `json:"unrealized_pnl"`
			} `json:"positions"`
		} `json:"accounts"`
	}
	params := map[string]string{
		"by":    "index",
		"value": strconv.FormatInt(c.cfg.AccountIndex, 10),
	}
	if err := c.getJSON(ctx, "api/v1/account", params, &resp); err != nil {
		return domain.AccountSnapshot{}, err
	}
	pos := domain.Position{}
	if len(resp.Accounts) > 0 {
		for _, p := range resp.Accounts[0].Positions {
			mid := p.MarketID
			if mid == 0 {
				mid = p.MarketIndex
			}
			if mid != c.marketIndex {
				continue
			}
			size, _ := strconv.ParseFloat(p.Position, 64)
			if p.Sign < 0 {
				size = -size
			}
			pos.SizeBTC = size
			pos.EntryPrice, _ = strconv.ParseFloat(p.AvgEntryPrice, 64)
			pos.Unrealized, _ = strconv.ParseFloat(p.UnrealizedPnl, 64)
		}
	}
	allowed := position.ComputeAllowed(pos, c.cfg.OrderSizeBTC, c.cfg.MaxPositionBTC)
	return domain.AccountSnapshot{Position: pos, Allowed: allowed}, nil
}

func (c *Client) HasPendingBotOrders(ctx context.Context) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.coiSet) > 0
}

func (c *Client) CancelBotOrders(ctx context.Context, symbol string) error {
	if c.signer == nil {
		c.mu.Lock()
		c.coiSet = make(map[int64]struct{})
		c.mu.Unlock()
		return nil
	}
	c.mu.Lock()
	coIDs := make([]int64, 0, len(c.coiSet))
	for id := range c.coiSet {
		coIDs = append(coIDs, id)
	}
	c.mu.Unlock()
	for _, id := range coIDs {
		if err := c.signer.CancelOrder(ctx, c.marketIndex, id); err != nil {
			return err
		}
		c.mu.Lock()
		delete(c.coiSet, id)
		c.mu.Unlock()
	}
	return nil
}

func (c *Client) PlaceLimitPostOnly(ctx context.Context, symbol string, tickID int64, intent domain.OrderIntent) (domain.OrderResult, error) {
	if intent.Skip {
		return domain.OrderResult{Status: "skipped"}, nil
	}
	if intent.SizeBTC < c.minBase-1e-12 {
		return domain.OrderResult{Status: "error", Error: "size below min_base"}, nil
	}
	c.mu.Lock()
	c.nextCOI++
	coi := c.nextCOI
	c.coiSet[coi] = struct{}{}
	c.mu.Unlock()

	isAsk := uint8(0)
	if intent.Side == domain.SideShort {
		isAsk = 1
	}
	reduceOnly := uint8(0)
	if intent.ReduceOnly {
		reduceOnly = 1
	}
	baseAmount := scaleSize(intent.SizeBTC, c.sizeDecimals)
	priceU := scalePrice(intent.Price, c.priceDecimals)
	expiry := time.Now().Add(10 * time.Minute).UnixMilli()

	if c.cfg.DryRun || c.signer == nil {
		return domain.OrderResult{ClientOrderIndex: coi, Status: "sim"}, nil
	}
	if err := c.signer.CreateLimitPostOnly(ctx, signer.CreateOrderParams{
		MarketIndex:      c.marketIndex,
		ClientOrderIndex: coi,
		BaseAmount:       baseAmount,
		Price:            priceU,
		IsAsk:            isAsk,
		ReduceOnly:       reduceOnly,
		OrderExpiry:      expiry,
	}); err != nil {
		c.mu.Lock()
		delete(c.coiSet, coi)
		c.mu.Unlock()
		return domain.OrderResult{Status: "error", Error: err.Error()}, nil
	}
	return domain.OrderResult{ClientOrderIndex: coi, Status: "placed"}, nil
}

func scaleSize(human float64, decimals int) int64 {
	scale := math.Pow10(decimals)
	return int64(math.Round(human * scale))
}

func scalePrice(human float64, decimals int) uint32 {
	scale := math.Pow10(decimals)
	return uint32(math.Round(human * scale))
}

func (c *Client) getJSON(ctx context.Context, path string, params map[string]string, out any) error {
	u, err := url.Parse(strings.TrimRight(c.cfg.LighterHost, "/") + "/" + path)
	if err != nil {
		return err
	}
	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("lighter %s: %s", path, string(body))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return err
	}
	return nil
}
