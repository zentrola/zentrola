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

func TestProrateUsesFixedPointRounding(t *testing.T) {
	for _, tt := range []struct {
		name               string
		amount             string
		numerator, divisor int64
		want               string
		wantErr            bool
	}{
		{name: "whole period", amount: "1000", numerator: 30, divisor: 30, want: "1000"},
		{name: "half period", amount: "1000", numerator: 1, divisor: 2, want: "500"},
		{name: "rounds half up", amount: "0.00000001", numerator: 1, divisor: 2, want: "0.00000001"},
		{name: "rounds down", amount: "0.00000001", numerator: 1, divisor: 3, want: "0"},
		{name: "repeating fraction", amount: "365", numerator: 31, divisor: 365, want: "31"},
		{name: "third", amount: "100", numerator: 1, divisor: 3, want: "33.33333333"},
		{name: "negative amount", amount: "-100", numerator: 2, divisor: 3, want: "-66.66666667"},
		{name: "zero overlap", amount: "100", numerator: 0, divisor: 3, want: "0"},
		{name: "ratio above one", amount: "100", numerator: 4, divisor: 3, wantErr: true},
		{name: "zero divisor", amount: "100", numerator: 0, divisor: 0, wantErr: true},
		{name: "invalid amount", amount: "1.123456789", numerator: 1, divisor: 2, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Prorate(tt.amount, tt.numerator, tt.divisor)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("Prorate()=%q err=%v; want %q wantErr=%v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}
