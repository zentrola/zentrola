package billing

import "testing"

func TestAllocatePreservesTotalAndUsesStableRemainder(t *testing.T) {
	total, items, err := Allocate("710", []UsageShare{
		{PrincipalID: 30, Tokens: 1},
		{PrincipalID: 10, Tokens: 1},
		{PrincipalID: 20, Tokens: 1},
	})
	if err != nil || total != 3 || len(items) != 3 {
		t.Fatalf("total=%d items=%v err=%v", total, items, err)
	}
	if items[0].PrincipalID != 10 || items[0].Amount != "236.66666667" ||
		items[1].PrincipalID != 20 || items[1].Amount != "236.66666667" ||
		items[2].PrincipalID != 30 || items[2].Amount != "236.66666666" {
		t.Fatalf("unexpected stable allocation: %+v", items)
	}
}

func TestAllocateSupportsAdjustmentsAndAggregatesPrincipals(t *testing.T) {
	total, items, err := Allocate("-200", []UsageShare{
		{PrincipalID: 1, Tokens: 60},
		{PrincipalID: 2, Tokens: 30},
		{PrincipalID: 1, Tokens: 10},
	})
	if err != nil || total != 100 || len(items) != 2 {
		t.Fatalf("total=%d items=%v err=%v", total, items, err)
	}
	if items[0].PrincipalID != 1 || items[0].Amount != "-140" || items[1].Amount != "-60" {
		t.Fatalf("unexpected adjustment allocation: %+v", items)
	}
}

func TestAllocateKeepsUnallocatedAmountWhenUsageIsZero(t *testing.T) {
	total, items, err := Allocate("800", []UsageShare{{PrincipalID: 1, Tokens: 0}})
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("total=%d items=%v err=%v", total, items, err)
	}
}

func TestSubtractAmounts(t *testing.T) {
	got, err := Subtract("800", "1000")
	if err != nil || got != "-200" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestAddAmountsPreservesFixedPrecision(t *testing.T) {
	got, err := Add("0.00000001", "999.99999999")
	if err != nil || got != "1000" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}
