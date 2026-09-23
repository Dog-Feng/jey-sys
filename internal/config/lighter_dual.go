package config

import (
	"errors"
	"fmt"
	"os"
)

// LighterLeg holds credentials for one Lighter account (dual mode).
type LighterLeg struct {
	AccountIndex  int64
	APIKeyIndex   uint8
	APIPrivateKey string
}

func loadLighterDual(c *Config) error {
	c.LighterDual = envBool("LIGHTER_DUAL", false)
	if !c.LighterDual {
		return nil
	}

	aIdx := int64(envInt("LIGHTER_ACCOUNT_A_INDEX", 0))
	if aIdx == 0 {
		aIdx = c.AccountIndex
	}
	bIdx := int64(envInt("LIGHTER_ACCOUNT_B_INDEX", 0))

	aKey := os.Getenv("LIGHTER_A_API_PRIVATE_KEY")
	if aKey == "" {
		aKey = c.APIPrivateKey
	}
	bKey := os.Getenv("LIGHTER_B_API_PRIVATE_KEY")

	aKeyIdx := uint8(envInt("LIGHTER_A_API_KEY_INDEX", int(c.APIKeyIndex)))
	bKeyIdx := uint8(envInt("LIGHTER_B_API_KEY_INDEX", int(c.APIKeyIndex)))

	c.LighterLegA = LighterLeg{AccountIndex: aIdx, APIKeyIndex: aKeyIdx, APIPrivateKey: aKey}
	c.LighterLegB = LighterLeg{AccountIndex: bIdx, APIKeyIndex: bKeyIdx, APIPrivateKey: bKey}

	if c.Exchange != "lighter" {
		return fmt.Errorf("LIGHTER_DUAL requires EXCHANGE=lighter (got %s)", c.Exchange)
	}
	if c.LighterLegA.AccountIndex <= 0 || c.LighterLegB.AccountIndex <= 0 {
		return errors.New("LIGHTER_DUAL requires LIGHTER_ACCOUNT_A_INDEX and LIGHTER_ACCOUNT_B_INDEX (or LIGHTER_ACCOUNT_INDEX for A)")
	}
	if c.LighterLegA.AccountIndex == c.LighterLegB.AccountIndex {
		return fmt.Errorf("LIGHTER dual: account A and B must differ (both %d)", c.LighterLegA.AccountIndex)
	}
	if !c.DryRun {
		if c.LighterLegA.APIPrivateKey == "" || c.LighterLegB.APIPrivateKey == "" {
			return errors.New("LIGHTER_DUAL live requires LIGHTER_A_API_PRIVATE_KEY and LIGHTER_B_API_PRIVATE_KEY (A may fall back to LIGHTER_API_PRIVATE_KEY)")
		}
	}
	loadDualUnwind(c)
	return nil
}

func loadDualUnwind(c *Config) {
	if !c.LighterDual {
		c.LighterDualUnwind = false
		return
	}
	c.LighterDualUnwind = envBool("LIGHTER_DUAL_UNWIND", true)
	c.UnwindEnterConfirmTicks = envInt("UNWIND_ENTER_CONFIRM_TICKS", 2)
	c.UnwindExitConfirmTicks = envInt("UNWIND_EXIT_CONFIRM_TICKS", 2)
	c.UnwindFlatEps = envFloat("UNWIND_FLAT_EPS", 1e-6)
	if c.UnwindEnterConfirmTicks < 1 {
		c.UnwindEnterConfirmTicks = 1
	}
	if c.UnwindExitConfirmTicks < 1 {
		c.UnwindExitConfirmTicks = 1
	}
}

func (c Config) WithLighterLeg(leg LighterLeg) Config {
	out := c
	out.AccountIndex = leg.AccountIndex
	out.APIKeyIndex = leg.APIKeyIndex
	out.APIPrivateKey = leg.APIPrivateKey
	return out
}

// EnvInt exported for tests — kept private; dual loader uses envInt only.
