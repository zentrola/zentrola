package billing

import "testing"

func TestRateAPIKeyUsageUsesFourteenDigitFixedPoint(t *testing.T) {
	cost, err := RateAPIKeyUsage(5, 0, 0, 0, "5", "1", "", "10")
	if err != nil {
		t.Fatal(err)
	}
	if cost.Input != "0.000025" || cost.CachedInput != "0" || cost.Output != "0" || cost.Total != "0.000025" {
		t.Fatalf("unexpected cost: %+v", cost)
	}
}

func TestRateAPIKeyUsageSeparatesCachedInput(t *testing.T) {
	cost, err := RateAPIKeyUsage(100, 40, 0, 20, "2", "0.5", "", "8")
	if err != nil {
		t.Fatal(err)
	}
	if cost.Input != "0.00012" || cost.CachedInput != "0.00002" || cost.Output != "0.00016" || cost.Total != "0.0003" {
		t.Fatalf("unexpected cost: %+v", cost)
	}
}

func TestRoundRatedCostsDistributesDeterministicRemainder(t *testing.T) {
	tokens, amount, allocations, err := RoundRatedCosts([]RatedShare{
		{PrincipalID: 2, Tokens: 1, Amount: "0.000000005"},
		{PrincipalID: 1, Tokens: 1, Amount: "0.000000005"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if tokens != 2 || amount != "0.00000001" || len(allocations) != 2 ||
		allocations[0].PrincipalID != 1 || allocations[0].Amount != "0.00000001" ||
		allocations[1].PrincipalID != 2 || allocations[1].Amount != "0" {
		t.Fatalf("tokens=%d amount=%s allocations=%+v", tokens, amount, allocations)
	}
}

func TestRateAPIKeyUsageRejectsCachedTokensAboveInput(t *testing.T) {
	if _, err := RateAPIKeyUsage(4, 5, 0, 0, "1", "1", "", "1"); err == nil {
		t.Fatal("invalid cached token count accepted")
	}
}

func TestRateAPIKeyUsageSeparatesCacheCreationInput(t *testing.T) {
	tests := []struct {
		name                                        string
		cacheCreationPrice                          string
		input, cacheCreation, cached, output, total string
	}{
		{
			name: "configured price", cacheCreationPrice: "3.75",
			input: "0.00015", cacheCreation: "0.00015", cached: "0.000006", output: "0.00015", total: "0.000456",
		},
		{
			// 未配置缓存写入单价时按普通输入单价计费，总成本与不区分缓存写入时一致。
			name: "fallback to input price", cacheCreationPrice: "",
			input: "0.00015", cacheCreation: "0.00012", cached: "0.000006", output: "0.00015", total: "0.000426",
		},
		{
			name: "free cache creation", cacheCreationPrice: "0",
			input: "0.00015", cacheCreation: "0", cached: "0.000006", output: "0.00015", total: "0.000306",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// 总输入 110 = 普通输入 50 + 缓存命中 20 + 缓存写入 40。
			cost, err := RateAPIKeyUsage(110, 20, 40, 10, "3", "0.3", test.cacheCreationPrice, "15")
			if err != nil {
				t.Fatal(err)
			}
			got := RatedCost{Input: test.input, CachedInput: test.cached, CacheCreationInput: test.cacheCreation,
				Output: test.output, Total: test.total}
			if cost != got {
				t.Fatalf("cost=%+v want=%+v", cost, got)
			}
		})
	}
}

func TestRateAPIKeyUsageRejectsInvalidCacheCreation(t *testing.T) {
	tests := []struct {
		name                         string
		input, cached, cacheCreation int64
		cacheCreationPrice           string
	}{
		{name: "negative tokens", input: 10, cacheCreation: -1},
		{name: "above uncached input", input: 10, cached: 6, cacheCreation: 5},
		{name: "negative price", input: 10, cacheCreation: 1, cacheCreationPrice: "-1"},
		{name: "malformed price", input: 10, cacheCreation: 1, cacheCreationPrice: "1.2.3"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := RateAPIKeyUsage(test.input, test.cached, test.cacheCreation, 0, "1", "1", test.cacheCreationPrice, "1"); err == nil {
				t.Fatal("invalid cache creation usage accepted")
			}
		})
	}
}
