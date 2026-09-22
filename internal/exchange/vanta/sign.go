package vanta

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

)

func decodeOrderlySecret(secret string) (ed25519.PrivateKey, string, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, "", fmt.Errorf("empty orderly secret")
	}
	payload := secret
	if strings.HasPrefix(secret, "ed25519:") {
		payload = strings.TrimPrefix(secret, "ed25519:")
	}
	raw, err := base58Decode(payload)
	if err != nil {
		return nil, "", fmt.Errorf("decode orderly secret: %w", err)
	}
	switch len(raw) {
	case ed25519.SeedSize:
		sk := ed25519.NewKeyFromSeed(raw)
		return sk, orderlyKeyFromPrivate(sk), nil
	case ed25519.PrivateKeySize:
		sk := ed25519.PrivateKey(raw)
		return sk, orderlyKeyFromPrivate(sk), nil
	default:
		return nil, "", fmt.Errorf("unexpected orderly secret length %d", len(raw))
	}
}

func orderlyKeyFromPrivate(sk ed25519.PrivateKey) string {
	pub := sk.Public().(ed25519.PublicKey)
	return "ed25519:" + base58Encode(pub)
}

func signOrderly(sk ed25519.PrivateKey, method, path, body string) (timestamp int64, sig string, err error) {
	ts := time.Now().UnixMilli()
	msg := fmt.Sprintf("%d%s%s%s", ts, strings.ToUpper(method), path, body)
	signature := ed25519.Sign(sk, []byte(msg))
	sig = base64.RawURLEncoding.EncodeToString(signature)
	return ts, sig, nil
}
