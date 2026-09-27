package policy

import (
	"testing"
	"time"
)

func TestLeaseTTL(t *testing.T) {
	def := 2 * time.Hour
	cases := map[string]time.Duration{"": def, "off": 0, "0": 0, "45m": 45 * time.Minute, "garbage": def, "-1h": def}
	for in, want := range cases {
		if got := (&PolicyConfig{ClaimLease: in}).LeaseTTL(def); got != want {
			t.Errorf("LeaseTTL(%q) = %v, want %v", in, got, want)
		}
	}
	var nilCfg *PolicyConfig
	if nilCfg.LeaseTTL(def) != def {
		t.Error("nil config should use the default")
	}
}
