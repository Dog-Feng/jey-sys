package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Symbol           string
	TickInterval     time.Duration
	OrderSizeBTC     float64
	MaxPositionBTC   float64
	QuoteInsideTicks int
	HorizonTicks     int
	MinSpreadBps     float64
	JevMinConfidence float64

	Exchange string // mock | lighter | vanta
	DryRun   bool

	VantaBaseURL       string
	VantaSymbol        string
	VantaAccountID     string
	VantaOrderlyKey    string
	VantaOrderlySecret string

	LighterHost       string
	LighterChainID    uint32
	AccountIndex      int64
	APIKeyIndex       uint8
	APIPrivateKey       string
	LighterLeverage     int  // 0 = do not send update-leverage tx at startup
	LighterLeverageCross bool // cross vs isolated margin

	Model            string // mock | jev
	TypeSafeAPIKey     string // active key (TypeSafeKeys[TypeSafeKeyIndex])
	TypeSafeKeys       []string
	TypeSafeKeyIndex   int
	JevModelID       string
	JevTimeout       time.Duration
	TypeSafeBaseURL  string

	DataDir          string
	HTTPListen       string
	OpsToken         string
}

func Load() (Config, error) {
	loadEnvFiles()
	c := Config{
		Symbol:           env("SYMBOL", "BTC"),
		TickInterval:     envDuration("TICK_INTERVAL", 2*time.Second),
		OrderSizeBTC:     envFloat("ORDER_SIZE_BTC", 0.001),
		MaxPositionBTC:   envFloat("MAX_POSITION_BTC", 0.003),
		QuoteInsideTicks: envInt("QUOTE_INSIDE_TICKS", 1),
		HorizonTicks:     envInt("HORIZON_TICKS", 15),
		MinSpreadBps:     envFloat("MIN_SPREAD_BPS", 0),
		JevMinConfidence: envFloat("JEV_MIN_CONFIDENCE", 0),

		Exchange: env("EXCHANGE", "mock"),
		DryRun:   envBool("DRY_RUN", true),

		VantaBaseURL:       env("VANTA_BASE_URL", "https://api.orderly.org"),
		VantaSymbol:        env("VANTA_SYMBOL", ""),
		VantaAccountID:     os.Getenv("VANTA_ORDERLY_ACCOUNT_ID"),
		VantaOrderlyKey:    os.Getenv("VANTA_ORDERLY_KEY"),
		VantaOrderlySecret: os.Getenv("VANTA_ORDERLY_SECRET"),

		LighterHost:    env("LIGHTER_HOST", "https://api.rh.lighter.xyz"),
		LighterChainID: uint32(envInt("LIGHTER_CHAIN_ID", 466324)),
		AccountIndex:   int64(envInt("LIGHTER_ACCOUNT_INDEX", 0)),
		APIKeyIndex:    uint8(envInt("LIGHTER_API_KEY_INDEX", 0)),
		APIPrivateKey:        os.Getenv("LIGHTER_API_PRIVATE_KEY"),
		LighterLeverage:      loadLighterLeverage(),
		LighterLeverageCross: envBool("LIGHTER_LEVERAGE_CROSS", true),

		Model: env("MODEL", "mock"),
		JevModelID:      env("JEV_MODEL_ID", "jev-1.13.0"),
		JevTimeout:      envDuration("JEV_TIMEOUT", 800*time.Millisecond),
		TypeSafeBaseURL: env("TYPESAFE_BASE_URL", "https://api.typesafe.ai"),

		DataDir:    env("DATA_DIR", "data"),
		HTTPListen: env("HTTP_LISTEN", "127.0.0.1:8080"),
		OpsToken:   os.Getenv("OPS_TOKEN"),
	}

	if c.OrderSizeBTC <= 0 {
		return c, errors.New("ORDER_SIZE_BTC must be > 0")
	}
	if c.MaxPositionBTC < c.OrderSizeBTC {
		return c, fmt.Errorf("MAX_POSITION_BTC (%v) must be >= ORDER_SIZE_BTC (%v)", c.MaxPositionBTC, c.OrderSizeBTC)
	}
	if c.Exchange == "lighter" && !c.DryRun {
		if c.AccountIndex <= 0 {
			return c, errors.New("LIGHTER_ACCOUNT_INDEX required when DRY_RUN=false")
		}
		if c.APIPrivateKey == "" {
			return c, errors.New("LIGHTER_API_PRIVATE_KEY required when DRY_RUN=false")
		}
	}
	if c.Exchange == "vanta" && !c.DryRun {
		if c.VantaAccountID == "" || c.VantaOrderlySecret == "" {
			return c, errors.New("VANTA_ORDERLY_ACCOUNT_ID and VANTA_ORDERLY_SECRET required when EXCHANGE=vanta and DRY_RUN=false")
		}
	}
	c.TypeSafeKeys, c.TypeSafeKeyIndex = loadTypeSafeKeys(c.Exchange)
	if c.Model == "jev" && len(c.TypeSafeKeys) == 0 {
		return c, errors.New("TYPESAFE_API_KEYS or TYPESAFE_API_KEY required when MODEL=jev")
	}
	for i, k := range c.TypeSafeKeys {
		k = strings.TrimSpace(k)
		c.TypeSafeKeys[i] = k
		if err := validateTypeSafeKey(k); err != nil {
			return c, fmt.Errorf("typesafe key #%d: %w", i+1, err)
		}
	}
	if len(c.TypeSafeKeys) > 0 {
		if c.TypeSafeKeyIndex < 0 || c.TypeSafeKeyIndex >= len(c.TypeSafeKeys) {
			c.TypeSafeKeyIndex = 0
		}
		c.TypeSafeAPIKey = c.TypeSafeKeys[c.TypeSafeKeyIndex]
	}
	if c.Model == "mock" && c.Exchange == "lighter" && !c.DryRun {
		// allow mock model on live exchange for testing execution path only
	}
	if c.LighterLeverage < 0 || c.LighterLeverage > 100 {
		return c, fmt.Errorf("LIGHTER_LEVERAGE / LEVERAGE must be 0 (skip) or 1–100, got %d", c.LighterLeverage)
	}
	return c, nil
}

// loadLighterLeverage reads LIGHTER_LEVERAGE, then legacy LEVERAGE. 0 skips startup leverage tx.
func loadLighterLeverage() int {
	if v := strings.TrimSpace(os.Getenv("LIGHTER_LEVERAGE")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0
		}
		return n
	}
	return envInt("LEVERAGE", 0)
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envFloat(k string, def float64) float64 {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

func envBool(k string, def bool) bool {
	v := strings.ToLower(os.Getenv(k))
	if v == "" {
		return def
	}
	return v == "1" || v == "true" || v == "yes"
}

func validateTypeSafeKey(key string) error {
	switch {
	case strings.HasPrefix(key, "vck_"):
		return errors.New("TYPESAFE_API_KEY looks like a Vercel AI Gateway key (vck_…); it cannot call https://api.typesafe.ai — create a key at https://console.typesafe.ai/keys or use Vercel Gateway with a different client (see README)")
	case strings.HasPrefix(key, "sk-or-"):
		return errors.New("TYPESAFE_API_KEY looks like an OpenRouter key; use a TypeSafe console key for api.typesafe.ai")
	default:
		return nil
	}
}

func envDuration(k string, def time.Duration) time.Duration {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
