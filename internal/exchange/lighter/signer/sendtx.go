package signer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/elliottech/lighter-go/types/txtypes"
)

func postSendTx(ctx context.Context, httpClient *http.Client, baseURL string, tx txtypes.TxInfo) (string, error) {
	txInfo, err := tx.GetTxInfo()
	if err != nil {
		return "", fmt.Errorf("tx GetTxInfo: %w", err)
	}
	form := url.Values{}
	form.Set("tx_type", strconv.Itoa(int(tx.GetTxType())))
	form.Set("tx_info", txInfo)
	form.Set("price_protection", "true")

	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/sendTx"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("sendTx http %d: %s", resp.StatusCode, string(body))
	}
	var out struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		TxHash  string `json:"tx_hash"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	if out.Code != 200 {
		return "", fmt.Errorf("sendTx code %d: %s", out.Code, out.Message)
	}
	return out.TxHash, nil
}
