// Package billing 编排个人订阅与 API Key 的用量核算、账期结算、差额调整和费用统计。
package billing

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	domain "github.com/zentrola/zentrola/internal/domain/billing"
	"github.com/zentrola/zentrola/internal/domain/shared"
)

const maxPeriodsPerScan = 10000

// errBillingID 表示无法生成计费记录 ID；属于全局依赖故障，出现时中止本轮而不是逐条计入失败。
var errBillingID = errors.New("cannot generate billing record ID")

type SubscriptionPrice struct {
	ID, CredentialID                      int64
	Currency, PeriodAmount, BillingPeriod string
	EffectiveAt, CredentialEffectiveAt    time.Time
	CredentialEndAt                       *time.Time
}

type ChargePeriod struct {
	CredentialID           int64
	PeriodStart, PeriodEnd time.Time
}

type Correction struct {
	OriginalDocumentID          int64
	CredentialID, SourcePriceID int64
	PeriodStart, PeriodEnd      time.Time
	Currency, TargetCurrency    string
	CurrentAmount, TargetAmount string
}

type DocumentItem struct {
	ID, PrincipalID int64
	UsageTokens     int64
	AllocationRatio string
	Amount          string
}

type Document struct {
	ID, CredentialID, SourceID, OriginalDocumentID int64
	BillingType, DocumentType, Status, SourceType  string
	PeriodStart, PeriodEnd, CreatedAt              time.Time
	TotalTokens                                    int64
	TotalAmount, Currency                          string
	Items                                          []DocumentItem
}

type StatisticsFilter struct {
	BillingType string
	From, To    time.Time
	After       int64
	Limit       int32
}

type CurrencyTotal struct {
	BillingType       string `json:"billingType"`
	Currency          string `json:"currency"`
	TotalAmount       string `json:"totalAmount"`
	AllocatedAmount   string `json:"allocatedAmount"`
	UnallocatedAmount string `json:"unallocatedAmount"`
	TotalTokens       int64  `json:"totalTokens"`
}

type PrincipalTotal struct {
	PrincipalID   int64  `json:"principalId,string"`
	PrincipalName string `json:"principalName"`
	PrincipalType string `json:"principalType"`
	BillingType   string `json:"billingType"`
	Currency      string `json:"currency"`
	Amount        string `json:"amount"`
	Tokens        int64  `json:"tokens"`
}

type Statistics struct {
	From   time.Time        `json:"from"`
	To     time.Time        `json:"to"`
	Totals []CurrencyTotal  `json:"totals"`
	Items  []PrincipalTotal `json:"items"`
	Total  int64            `json:"total"`
}

// Summary 汇总一次结算扫描。单个账期或调整失败只计入 Failed 并继续处理其他账期；
// FailureSamples 保留前几条已脱敏的失败描述，供任务日志定位问题。
type Summary struct {
	Created, Adjusted, Skipped, Failed int
	FailureSamples                     []string
}

// maxFailureSamples 限制单轮保留的失败描述数量，避免同一批坏数据刷屏。
const maxFailureSamples = 5

func (s *Summary) recordFailure(format string, args ...any) {
	s.Failed++
	if len(s.FailureSamples) < maxFailureSamples {
		s.FailureSamples = append(s.FailureSamples, fmt.Sprintf(format, args...))
	}
}

// StoreError 是存储适配器返回的已脱敏错误：只保留操作名、SQLSTATE、约束名和数据库报错文本，
// 不包含 SQL 参数、连接信息或凭据，可以安全写入日志。
type StoreError struct {
	Operation  string
	SQLState   string
	Constraint string
	Message    string
	Err        error
}

func (e *StoreError) Error() string {
	if e == nil {
		return "<nil>"
	}
	text := "billing store " + e.Operation + " failed"
	if e.SQLState != "" {
		text += ": sqlstate=" + e.SQLState
		if e.Constraint != "" {
			text += " constraint=" + e.Constraint
		}
		if e.Message != "" {
			text += " message=" + strconv.Quote(e.Message)
		}
		return text
	}
	switch {
	case errors.Is(e.Err, context.Canceled):
		return text + ": context canceled"
	case errors.Is(e.Err, context.DeadlineExceeded):
		return text + ": deadline exceeded"
	default:
		return text + fmt.Sprintf(": cause_type=%T", e.Err)
	}
}

func (e *StoreError) Unwrap() error { return e.Err }

// SafeErrorText 返回可写入日志的错误描述。存储错误只输出脱敏字段；未经存储适配器包装的
// 数据库错误只输出 SQLSTATE；其余错误来自计费领域与应用层的固定文本。
func SafeErrorText(err error) string {
	if err == nil {
		return ""
	}
	var storeErr *StoreError
	if errors.As(err, &storeErr) {
		return storeErr.Error()
	}
	var sqlErr interface{ SQLState() string }
	if errors.As(err, &sqlErr) {
		return "database error: sqlstate=" + sqlErr.SQLState()
	}
	return err.Error()
}

type Store interface {
	SubscriptionPrices(context.Context) ([]SubscriptionPrice, error)
	ChargePeriods(context.Context) ([]ChargePeriod, error)
	SubscriptionUsage(context.Context, int64, time.Time, time.Time) ([]domain.UsageShare, error)
	CreateCharge(context.Context, Document) (bool, error)
	SubscriptionCorrections(context.Context) ([]Correction, error)
	OriginalUsage(context.Context, int64) ([]domain.UsageShare, error)
	CreateAdjustment(context.Context, string, Document) (bool, error)
	UsageRatingCandidates(context.Context, RatingScan, int64, int32) ([]UsageRatingCandidate, error)
	CreateUsageRating(context.Context, UsageRating) (bool, error)
	APIKeyPeriods(context.Context, time.Time) ([]APIKeyPeriod, error)
	APIKeyUsage(context.Context, APIKeyPeriod) ([]domain.RatedShare, []int64, error)
	APIKeyCurrent(context.Context, APIKeyPeriod) (APIKeyCurrent, error)
	CreateAPIKeyDocument(context.Context, string, int64, Document, []int64) (bool, error)
	Statistics(context.Context, admin.Identity, StatisticsFilter) (Statistics, error)
	BillingDocuments(context.Context, DocumentFilter) (DocumentPage, error)
	BillingDocument(context.Context, int64) (DocumentDetail, error)
	UnratedUsage(context.Context, UnratedUsageFilter) (UnratedUsagePage, error)
}

type Service struct {
	store Store
	ids   shared.IDGenerator
	now   func() time.Time
}

func New(store Store, ids shared.IDGenerator) *Service {
	return &Service{store: store, ids: ids, now: func() time.Time { return time.Now().UTC() }}
}

// SettleDueSubscriptions 为已到期的个人订阅账期生成账单，并为价格金额纠错生成调整单。
// 单个账期失败只计入 Failed 后继续；读取价格或已出账账期失败、
// 上下文取消、ID 生成失败等全局错误才中止本轮。
func (s *Service) SettleDueSubscriptions(ctx context.Context, cutoff time.Time) (Summary, error) {
	var summary Summary
	if s == nil || s.store == nil || s.ids == nil || cutoff.IsZero() {
		return summary, appsec.ErrUnavailable
	}
	cutoff = cutoff.UTC()
	prices, err := s.store.SubscriptionPrices(ctx)
	if err != nil {
		return summary, err
	}
	periods, err := s.store.ChargePeriods(ctx)
	if err != nil {
		return summary, err
	}
	existing := make(map[periodKey]struct{}, len(periods))
	for _, period := range periods {
		existing[newPeriodKey(period.CredentialID, period.PeriodStart, period.PeriodEnd)] = struct{}{}
	}

	groups := groupSubscriptionPrices(prices)
	periodCount := 0
	for _, group := range groups {
		periodStart := group[0].EffectiveAt.UTC()
		if group[0].CredentialEffectiveAt.After(periodStart) {
			periodStart = group[0].CredentialEffectiveAt.UTC()
		}
		anchorDay := periodStart.Day()
		for !periodStart.After(cutoff) {
			if group[0].CredentialEndAt != nil && !periodStart.Before(group[0].CredentialEndAt.UTC()) {
				break
			}
			price := priceAt(group, periodStart)
			if price == nil {
				break
			}
			periodEnd, addErr := addBillingPeriod(periodStart, price.BillingPeriod, anchorDay)
			if addErr != nil {
				summary.recordFailure("subscription credential=%d price=%d: %v", price.CredentialID, price.ID, addErr)
				break
			}
			if periodEnd.After(cutoff) {
				break
			}
			key := newPeriodKey(price.CredentialID, periodStart, periodEnd)
			if _, found := existing[key]; found {
				summary.Skipped++
				periodStart = periodEnd
				continue
			}
			periodCount++
			if periodCount > maxPeriodsPerScan {
				return summary, errors.New("billing settlement scan exceeded period limit")
			}
			created, chargeErr := s.settleSubscriptionPeriod(ctx, price, periodStart, periodEnd)
			if chargeErr != nil {
				if ctx.Err() != nil {
					return summary, ctx.Err()
				}
				if errors.Is(chargeErr, errBillingID) {
					return summary, chargeErr
				}
				summary.recordFailure("subscription credential=%d period=%s/%s: %s", price.CredentialID,
					formatPeriodTime(periodStart), formatPeriodTime(periodEnd), SafeErrorText(chargeErr))
				periodStart = periodEnd
				continue
			}
			if created {
				summary.Created++
			} else {
				summary.Skipped++
			}
			existing[key] = struct{}{}
			periodStart = periodEnd
		}
	}

	corrections, err := s.store.SubscriptionCorrections(ctx)
	if err != nil {
		return summary, err
	}
	for _, correction := range corrections {
		if correction.Currency != correction.TargetCurrency {
			summary.recordFailure("subscription document=%d: currency changed from %s to %s",
				correction.OriginalDocumentID, correction.Currency, correction.TargetCurrency)
			continue
		}
		outcome, adjustErr := s.adjustSubscriptionDocument(ctx, correction)
		if adjustErr != nil {
			if ctx.Err() != nil {
				return summary, ctx.Err()
			}
			if errors.Is(adjustErr, errBillingID) {
				return summary, adjustErr
			}
			summary.recordFailure("subscription document=%d adjustment: %s",
				correction.OriginalDocumentID, SafeErrorText(adjustErr))
			continue
		}
		switch outcome {
		case adjustmentCreated:
			summary.Adjusted++
		case adjustmentSkipped:
			summary.Skipped++
		}
	}
	return summary, nil
}

func (s *Service) settleSubscriptionPeriod(ctx context.Context, price *SubscriptionPrice, periodStart, periodEnd time.Time) (bool, error) {
	usage, err := s.store.SubscriptionUsage(ctx, price.CredentialID, periodStart, periodEnd)
	if err != nil {
		return false, err
	}
	totalTokens, allocations, err := domain.Allocate(price.PeriodAmount, usage)
	if err != nil {
		return false, err
	}
	document, err := s.newDocument(ctx, "CHARGE", price.CredentialID, price.ID, 0,
		periodStart, periodEnd, totalTokens, price.PeriodAmount, price.Currency, allocations, true)
	if err != nil {
		return false, err
	}
	return s.store.CreateCharge(ctx, document)
}

type adjustmentOutcome int

const (
	adjustmentNone adjustmentOutcome = iota
	adjustmentCreated
	adjustmentSkipped
)

func (s *Service) adjustSubscriptionDocument(ctx context.Context, correction Correction) (adjustmentOutcome, error) {
	delta, err := domain.Subtract(correction.TargetAmount, correction.CurrentAmount)
	if err != nil {
		return adjustmentNone, err
	}
	if delta == "0" {
		return adjustmentNone, nil
	}
	usage, err := s.store.OriginalUsage(ctx, correction.OriginalDocumentID)
	if err != nil {
		return adjustmentNone, err
	}
	_, allocations, err := domain.Allocate(delta, usage)
	if err != nil {
		return adjustmentNone, err
	}
	document, err := s.newDocument(ctx, "ADJUSTMENT", correction.CredentialID,
		correction.SourcePriceID, correction.OriginalDocumentID, correction.PeriodStart,
		correction.PeriodEnd, 0, delta, correction.Currency, allocations, false)
	if err != nil {
		return adjustmentNone, err
	}
	created, err := s.store.CreateAdjustment(ctx, correction.CurrentAmount, document)
	if err != nil {
		return adjustmentNone, err
	}
	if created {
		return adjustmentCreated, nil
	}
	return adjustmentSkipped, nil
}

func formatPeriodTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339)
}

func (s *Service) Statistics(ctx context.Context, actor admin.Identity, filter StatisticsFilter) (Statistics, error) {
	if actor.ID <= 0 {
		return Statistics{}, appsec.ErrUnauthenticated
	}
	if s == nil || s.store == nil {
		return Statistics{}, appsec.ErrUnavailable
	}
	if filter.From.IsZero() || !filter.To.After(filter.From) || filter.To.Sub(filter.From) > 366*24*time.Hour ||
		filter.After < 0 || filter.Limit < 1 || filter.Limit > 100 ||
		filter.BillingType != "" && filter.BillingType != "SUBSCRIPTION" && filter.BillingType != "API_KEY" {
		return Statistics{}, appsec.ErrInvalidArgument
	}
	filter.From, filter.To = filter.From.UTC(), filter.To.UTC()
	return s.store.Statistics(ctx, actor, filter)
}

func (s *Service) newDocument(ctx context.Context, documentType string, credentialID, sourceID, originalID int64,
	periodStart, periodEnd time.Time, totalTokens int64, amount, currency string,
	allocations []domain.Allocation, keepUsage bool,
) (Document, error) {
	documentID, err := s.ids.NextID(ctx)
	if err != nil || documentID <= 0 {
		return Document{}, errBillingID
	}
	document := Document{
		ID: documentID, CredentialID: credentialID, SourceID: sourceID, OriginalDocumentID: originalID,
		BillingType: "SUBSCRIPTION", DocumentType: documentType, Status: "CONFIRMED", SourceType: "SUBSCRIPTION_PRICE",
		PeriodStart: periodStart.UTC(), PeriodEnd: periodEnd.UTC(), CreatedAt: s.now().UTC(),
		TotalTokens: totalTokens, TotalAmount: amount, Currency: currency, Items: make([]DocumentItem, 0, len(allocations)),
	}
	for _, allocation := range allocations {
		itemID, idErr := s.ids.NextID(ctx)
		if idErr != nil || itemID <= 0 {
			return Document{}, errBillingID
		}
		usageTokens := int64(0)
		if keepUsage {
			usageTokens = allocation.Tokens
		}
		document.Items = append(document.Items, DocumentItem{
			ID: itemID, PrincipalID: allocation.PrincipalID, UsageTokens: usageTokens,
			AllocationRatio: allocation.Ratio, Amount: allocation.Amount,
		})
	}
	return document, nil
}

type periodKey struct {
	credentialID int64
	start, end   int64
}

func newPeriodKey(credentialID int64, start, end time.Time) periodKey {
	return periodKey{credentialID: credentialID, start: start.UTC().UnixNano(), end: end.UTC().UnixNano()}
}

func groupSubscriptionPrices(prices []SubscriptionPrice) [][]SubscriptionPrice {
	prices = append([]SubscriptionPrice(nil), prices...)
	sort.Slice(prices, func(i, j int) bool {
		if prices[i].CredentialID != prices[j].CredentialID {
			return prices[i].CredentialID < prices[j].CredentialID
		}
		if !prices[i].EffectiveAt.Equal(prices[j].EffectiveAt) {
			return prices[i].EffectiveAt.Before(prices[j].EffectiveAt)
		}
		return prices[i].ID < prices[j].ID
	})
	groups := make([][]SubscriptionPrice, 0)
	for _, price := range prices {
		if len(groups) == 0 || groups[len(groups)-1][0].CredentialID != price.CredentialID {
			groups = append(groups, []SubscriptionPrice{price})
		} else {
			groups[len(groups)-1] = append(groups[len(groups)-1], price)
		}
	}
	return groups
}

func priceAt(prices []SubscriptionPrice, at time.Time) *SubscriptionPrice {
	var result *SubscriptionPrice
	for index := range prices {
		if prices[index].EffectiveAt.After(at) {
			break
		}
		result = &prices[index]
	}
	return result
}

func addBillingPeriod(start time.Time, period string, anchorDay int) (time.Time, error) {
	start = start.UTC()
	year, month, day := start.Date()
	switch period {
	case "MONTH":
		month++
		if month > 12 {
			year++
			month = 1
		}
	case "YEAR":
		year++
	default:
		return time.Time{}, errors.New("invalid subscription billing period")
	}
	lastDay := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
	day = anchorDay
	if day > lastDay {
		day = lastDay
	}
	return time.Date(year, month, day, start.Hour(), start.Minute(), start.Second(), start.Nanosecond(), time.UTC), nil
}
