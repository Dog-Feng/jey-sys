package lighter

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// flexFloat accepts JSON number or string (Lighter API often uses strings).
type flexFloat float64

func (f *flexFloat) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*f = 0
		return nil
	}
	var n float64
	if err := json.Unmarshal(b, &n); err == nil {
		*f = flexFloat(n)
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("flexFloat: %w", err)
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("flexFloat parse %q: %w", s, err)
	}
	*f = flexFloat(v)
	return nil
}
