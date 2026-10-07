// Package billing 定义计费单据分摊和金额运算规则。
package billing

import (
	"errors"
	"math"
	"math/big"
	"sort"
	"strings"
)

const amountScale = 8

type UsageShare struct {
	PrincipalID int64
	Tokens      int64
}

type Allocation struct {
	PrincipalID int64
	Tokens      int64
	Ratio       string
	Amount      string
}

// Allocate 按 Token 比例分配金额。金额使用 8 位定点小数，无法整除的最小单位
// 依次分配给 Token 较多、主体 ID 较小的记录，确保明细金额之和严格等于总额。
func Allocate(amount string, shares []UsageShare) (int64, []Allocation, error) {
	units, err := parseAmount(amount)
	if err != nil {
		return 0, nil, err
	}
	byPrincipal := make(map[int64]int64, len(shares))
	var totalTokens int64
	for _, share := range shares {
		if share.PrincipalID <= 0 || share.Tokens < 0 {
			return 0, nil, errors.New("invalid billing usage share")
		}
		if share.Tokens == 0 {
			continue
		}
		current := byPrincipal[share.PrincipalID]
		if share.Tokens > math.MaxInt64-current || share.Tokens > math.MaxInt64-totalTokens {
			return 0, nil, errors.New("billing token total overflow")
		}
		byPrincipal[share.PrincipalID] = current + share.Tokens
		totalTokens += share.Tokens
	}
	if totalTokens == 0 {
		return 0, []Allocation{}, nil
	}

	ordered := make([]UsageShare, 0, len(byPrincipal))
	for principalID, tokens := range byPrincipal {
		ordered = append(ordered, UsageShare{PrincipalID: principalID, Tokens: tokens})
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Tokens != ordered[j].Tokens {
			return ordered[i].Tokens > ordered[j].Tokens
		}
		return ordered[i].PrincipalID < ordered[j].PrincipalID
	})

	negative := units.Sign() < 0
	absolute := new(big.Int).Abs(new(big.Int).Set(units))
	denominator := big.NewInt(totalTokens)
	allocated := new(big.Int)
	result := make([]Allocation, len(ordered))
	for index, share := range ordered {
		product := new(big.Int).Mul(absolute, big.NewInt(share.Tokens))
		itemUnits := new(big.Int).Quo(product, denominator)
		allocated.Add(allocated, itemUnits)
		if negative {
			itemUnits.Neg(itemUnits)
		}
		result[index] = Allocation{
			PrincipalID: share.PrincipalID,
			Tokens:      share.Tokens,
			Ratio:       new(big.Rat).SetFrac(big.NewInt(share.Tokens), denominator).FloatString(16),
			Amount:      formatAmount(itemUnits),
		}
	}

	remainder := new(big.Int).Sub(absolute, allocated).Int64()
	for index := int64(0); index < remainder; index++ {
		itemUnits, parseErr := parseAmount(result[index].Amount)
		if parseErr != nil {
			return 0, nil, parseErr
		}
		if negative {
			itemUnits.Sub(itemUnits, big.NewInt(1))
		} else {
			itemUnits.Add(itemUnits, big.NewInt(1))
		}
		result[index].Amount = formatAmount(itemUnits)
	}
	return totalTokens, result, nil
}

// Subtract 返回 left-right，供历史账单生成差额调整。
func Subtract(left, right string) (string, error) {
	leftUnits, err := parseAmount(left)
	if err != nil {
		return "", err
	}
	rightUnits, err := parseAmount(right)
	if err != nil {
		return "", err
	}
	return formatAmount(new(big.Int).Sub(leftUnits, rightUnits)), nil
}

func parseAmount(value string) (*big.Int, error) {
	return parseFixed(value, amountScale)
}

func parseFixed(value string, scale int) (*big.Int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("empty billing amount")
	}
	negative := strings.HasPrefix(value, "-")
	if negative || strings.HasPrefix(value, "+") {
		value = value[1:]
	}
	parts := strings.Split(value, ".")
	if scale < 0 || len(parts) > 2 || parts[0] == "" || len(parts) == 2 && len(parts[1]) > scale {
		return nil, errors.New("invalid billing amount")
	}
	for _, part := range parts {
		if part == "" && len(parts) == 2 {
			return nil, errors.New("invalid billing amount")
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return nil, errors.New("invalid billing amount")
			}
		}
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	digits := strings.TrimLeft(parts[0]+fraction+strings.Repeat("0", scale-len(fraction)), "0")
	if digits == "" {
		digits = "0"
	}
	units, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return nil, errors.New("invalid billing amount")
	}
	if negative {
		units.Neg(units)
	}
	return units, nil
}

func formatAmount(units *big.Int) string {
	return formatFixed(units, amountScale)
}

func formatFixed(units *big.Int, scale int) string {
	negative := units.Sign() < 0
	digits := new(big.Int).Abs(new(big.Int).Set(units)).String()
	if len(digits) <= scale {
		digits = strings.Repeat("0", scale-len(digits)+1) + digits
	}
	value := digits
	if scale > 0 {
		value = digits[:len(digits)-scale] + "." + digits[len(digits)-scale:]
	}
	if scale > 0 {
		value = strings.TrimRight(strings.TrimRight(value, "0"), ".")
	}
	if value == "" {
		value = "0"
	}
	if negative && value != "0" {
		return "-" + value
	}
	return value
}
