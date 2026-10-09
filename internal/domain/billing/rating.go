package billing

import (
	"errors"
	"math"
	"math/big"
	"sort"
)

const ratingScale = 14

type RatedCost struct {
	Input, CachedInput, Output, Total string
}

type RatedShare struct {
	PrincipalID int64
	Tokens      int64
	Amount      string
}

// RateAPIKeyUsage 使用价格的 8 位定点整数计算单次调用成本。价格单位是每百万
// Token；因此价格最小单位乘以 Token 数后，恰好就是 14 位成本的最小单位。
func RateAPIKeyUsage(inputTokens, cachedInputTokens, outputTokens int64, inputPrice, cachedInputPrice, outputPrice string) (RatedCost, error) {
	if inputTokens < 0 || cachedInputTokens < 0 || cachedInputTokens > inputTokens || outputTokens < 0 {
		return RatedCost{}, errors.New("invalid api key token usage")
	}
	inputUnits, err := parseFixed(inputPrice, amountScale)
	if err != nil || inputUnits.Sign() < 0 {
		return RatedCost{}, errors.New("invalid api key input price")
	}
	cachedUnits, err := parseFixed(cachedInputPrice, amountScale)
	if err != nil || cachedUnits.Sign() < 0 {
		return RatedCost{}, errors.New("invalid api key cached input price")
	}
	outputUnits, err := parseFixed(outputPrice, amountScale)
	if err != nil || outputUnits.Sign() < 0 {
		return RatedCost{}, errors.New("invalid api key output price")
	}
	normalInput := inputTokens - cachedInputTokens
	inputCost := new(big.Int).Mul(inputUnits, big.NewInt(normalInput))
	cachedCost := new(big.Int).Mul(cachedUnits, big.NewInt(cachedInputTokens))
	outputCost := new(big.Int).Mul(outputUnits, big.NewInt(outputTokens))
	total := new(big.Int).Add(new(big.Int).Set(inputCost), cachedCost)
	total.Add(total, outputCost)
	return RatedCost{
		Input:       formatFixed(inputCost, ratingScale),
		CachedInput: formatFixed(cachedCost, ratingScale),
		Output:      formatFixed(outputCost, ratingScale),
		Total:       formatFixed(total, ratingScale),
	}, nil
}

// RoundRatedCosts 将 14 位的单次核算汇总为 8 位账单金额。先对总额四舍五入，
// 再按各主体被截断的余数从大到小分配最小金额单位，确保明细之和等于单据总额。
func RoundRatedCosts(shares []RatedShare) (int64, string, []Allocation, error) {
	type accumulated struct {
		principalID int64
		tokens      int64
		units       *big.Int
		remainder   *big.Int
		billUnits   *big.Int
	}
	byPrincipal := make(map[int64]*accumulated, len(shares))
	var totalTokens int64
	totalRatingUnits := new(big.Int)
	for _, share := range shares {
		if share.PrincipalID <= 0 || share.Tokens < 0 {
			return 0, "", nil, errors.New("invalid rated usage share")
		}
		units, err := parseFixed(share.Amount, ratingScale)
		if err != nil || units.Sign() < 0 {
			return 0, "", nil, errors.New("invalid rated usage amount")
		}
		if share.Tokens > math.MaxInt64-totalTokens {
			return 0, "", nil, errors.New("rated token total overflow")
		}
		totalTokens += share.Tokens
		totalRatingUnits.Add(totalRatingUnits, units)
		current := byPrincipal[share.PrincipalID]
		if current == nil {
			current = &accumulated{principalID: share.PrincipalID, units: new(big.Int)}
			byPrincipal[share.PrincipalID] = current
		}
		if share.Tokens > math.MaxInt64-current.tokens {
			return 0, "", nil, errors.New("rated principal token total overflow")
		}
		current.tokens += share.Tokens
		current.units.Add(current.units, units)
	}

	factor := big.NewInt(1_000_000)
	half := big.NewInt(500_000)
	totalBillUnits := new(big.Int).Quo(new(big.Int).Add(new(big.Int).Set(totalRatingUnits), half), factor)
	ordered := make([]*accumulated, 0, len(byPrincipal))
	allocated := new(big.Int)
	for _, current := range byPrincipal {
		current.billUnits, current.remainder = new(big.Int), new(big.Int)
		current.billUnits.QuoRem(current.units, factor, current.remainder)
		allocated.Add(allocated, current.billUnits)
		ordered = append(ordered, current)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if comparison := ordered[i].remainder.Cmp(ordered[j].remainder); comparison != 0 {
			return comparison > 0
		}
		return ordered[i].principalID < ordered[j].principalID
	})
	remainderUnits := new(big.Int).Sub(totalBillUnits, allocated)
	if !remainderUnits.IsInt64() || remainderUnits.Sign() < 0 || remainderUnits.Int64() > int64(len(ordered)) {
		return 0, "", nil, errors.New("invalid rated amount remainder")
	}
	for index := int64(0); index < remainderUnits.Int64(); index++ {
		ordered[index].billUnits.Add(ordered[index].billUnits, big.NewInt(1))
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].principalID < ordered[j].principalID })
	result := make([]Allocation, 0, len(ordered))
	for _, current := range ordered {
		result = append(result, Allocation{
			PrincipalID: current.principalID,
			Tokens:      current.tokens,
			Amount:      formatFixed(current.billUnits, amountScale),
		})
	}
	return totalTokens, formatFixed(totalBillUnits, amountScale), result, nil
}
