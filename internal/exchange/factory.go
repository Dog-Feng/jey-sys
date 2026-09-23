package exchange

import (
	"fmt"

	"github.com/jev-sys/bot/internal/config"
	"github.com/jev-sys/bot/internal/exchange/lighter"
	"github.com/jev-sys/bot/internal/exchange/mock"
	"github.com/jev-sys/bot/internal/exchange/vanta"
)

func New(cfg config.Config) (Exchange, error) {
	switch cfg.Exchange {
	case "mock":
		return mock.New(cfg), nil
	case "lighter":
		return lighter.New(cfg)
	case "vanta":
		return vanta.New(cfg)
	default:
		return nil, fmt.Errorf("unknown EXCHANGE: %s", cfg.Exchange)
	}
}
