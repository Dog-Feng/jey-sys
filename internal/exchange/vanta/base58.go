package vanta

import (
	"fmt"
	"math/big"
)

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

func base58Decode(s string) ([]byte, error) {
	if s == "" {
		return nil, fmt.Errorf("empty base58")
	}
	zeros := 0
	for zeros < len(s) && s[zeros] == '1' {
		zeros++
	}
	n := big.NewInt(0)
	radix := big.NewInt(58)
	for i := zeros; i < len(s); i++ {
		idx := -1
		for j := 0; j < len(base58Alphabet); j++ {
			if base58Alphabet[j] == s[i] {
				idx = j
				break
			}
		}
		if idx < 0 {
			return nil, fmt.Errorf("invalid base58 char %q", s[i])
		}
		n.Mul(n, radix)
		n.Add(n, big.NewInt(int64(idx)))
	}
	out := n.Bytes()
	if zeros > 0 {
		prefix := make([]byte, zeros)
		out = append(prefix, out...)
	}
	return out, nil
}

func base58Encode(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	zeros := 0
	for zeros < len(b) && b[zeros] == 0 {
		zeros++
	}
	n := new(big.Int).SetBytes(b)
	radix := big.NewInt(58)
	zero := big.NewInt(0)
	mod := new(big.Int)
	var encoded []byte
	for n.Cmp(zero) != 0 {
		n.DivMod(n, radix, mod)
		encoded = append(encoded, base58Alphabet[mod.Int64()])
	}
	for i, j := 0, len(encoded)-1; i < j; i, j = i+1, j-1 {
		encoded[i], encoded[j] = encoded[j], encoded[i]
	}
	out := make([]byte, zeros+len(encoded))
	for i := 0; i < zeros; i++ {
		out[i] = '1'
	}
	copy(out[zeros:], encoded)
	return string(out)
}
