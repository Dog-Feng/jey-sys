package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jev-sys/bot/internal/config"
	"github.com/jev-sys/bot/internal/domain"
)

type JevModel struct {
	cfg    config.Config
	client *http.Client
}

func NewJev(cfg config.Config) *JevModel {
	return &JevModel{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.JevTimeout,
		},
	}
}

type systemOneReq struct {
	State     any               `json:"state"`
	Model     string            `json:"model"`
	Questions map[string]any    `json:"questions"`
}

type systemOneResp struct {
	Answers map[string]struct {
		Type          string             `json:"type"`
		Choice        string             `json:"choice"`
		Probabilities map[string]float64 `json:"probabilities"`
		Confidence    float64            `json:"confidence"`
	} `json:"answers"`
}

func (j *JevModel) Decide(ctx context.Context, state domain.TradeState) (domain.Decision, error) {
	start := time.Now()
	body := systemOneReq{
		Model: j.cfg.JevModelID,
		State: state,
		Questions: map[string]any{
			"direction": map[string]any{
				"type":         "choice",
				"instructions": "Over the next horizon, is price more likely up or down vs mid?",
				"criteria": map[string]string{
					"buy":  "Upward bias",
					"sell": "Downward bias",
				},
			},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return domain.Decision{}, err
	}
	url := j.cfg.TypeSafeBaseURL + "/v1/systemone"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return domain.Decision{}, err
	}
	req.Header.Set("Authorization", "Bearer "+j.cfg.TypeSafeAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := j.client.Do(req)
	if err != nil {
		return domain.Decision{}, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return domain.Decision{}, fmt.Errorf("jev http %d: %s", resp.StatusCode, string(b))
	}
	var out systemOneResp
	if err := json.Unmarshal(b, &out); err != nil {
		return domain.Decision{}, err
	}
	dir := out.Answers["direction"]
	action := domain.ActionBuy
	if dir.Choice == "sell" {
		action = domain.ActionSell
	}
	probs := map[domain.Action]float64{
		domain.ActionBuy:  dir.Probabilities["buy"],
		domain.ActionSell: dir.Probabilities["sell"],
	}
	return domain.Decision{
		Action:        action,
		Probabilities: probs,
		Confidence:    dir.Confidence,
		LatencyMs:     time.Since(start).Milliseconds(),
	}, nil
}
