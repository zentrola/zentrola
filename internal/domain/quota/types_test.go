package quota

import (
	"testing"
	"time"
)

func TestNewStatusLevelsAndZeroLimit(t *testing.T) {
	at := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name      string
		limit     int64
		used      int64
		level     string
		remaining int64
	}{
		{name: "normal", limit: 100, used: 49, level: "NORMAL", remaining: 51},
		{name: "notice", limit: 100, used: 50, level: "NOTICE", remaining: 50},
		{name: "warning", limit: 100, used: 80, level: "WARNING", remaining: 20},
		{name: "exhausted", limit: 100, used: 100, level: "EXHAUSTED", remaining: 0},
		{name: "defensive zero limit", limit: 0, used: 0, level: "EXHAUSTED", remaining: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			status := NewStatus(Scope{Type: Principal, ID: 7, Limit: test.limit}, test.used, at)
			if status.Level != test.level || status.RemainingTokens != test.remaining {
				t.Fatalf("status=%+v", status)
			}
			if !status.PeriodStart.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) ||
				!status.PeriodEnd.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) {
				t.Fatalf("period=%s..%s", status.PeriodStart, status.PeriodEnd)
			}
		})
	}
}
