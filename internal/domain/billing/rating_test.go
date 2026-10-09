package billing

import "testing"

func TestRateAPIKeyUsageUsesFourteenDigitFixedPoint(t *testing.T) {
	cost, err := RateAPIKeyUsage(5, 0, 0, "5", "1", "10")
	if err != nil {
		t.Fatal(err)
	}
	if cost.Input != "0.000025" || cost.CachedInput != "0" || cost.Output != "0" || cost.Total != "0.000025" {
		t.Fatalf("unexpected cost: %+v", cost)
	}
}

func TestRateAPIKeyUsageSeparatesCachedInput(t *testing.T) {
	cost, err := RateAPIKeyUsage(100, 40, 20, "2", "0.5", "8")
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
	if _, err := RateAPIKeyUsage(4, 5, 0, "1", "1", "1"); err == nil {
		t.Fatal("invalid cached token count accepted")
	}
}
