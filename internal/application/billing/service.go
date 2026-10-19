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

// ErrSubscriptionPricesChanged 表示出账事务获取锁后发现账期起点及之前的价格历史与结算读取时不一致：
// 价格写入在结算读取价格之后提交，账期锚点或起点有效价格可能已变化。本轮放弃该凭据的出账，由下一轮重新计算。
var ErrSubscriptionPricesChanged = errors.New("subscription prices changed since settlement read")

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
	// CreateCharge 写入订阅账期单据。prices 是结算计算该账期所依据的价格历史（生效时间不晚于账期起点，
	// 按生效时间和 ID 升序，最后一条为来源价格）；实现必须在与价格写入串行化的锁内确认其未变化，
	// 变化时不写入并返回 ErrSubscriptionPricesChanged。
	CreateCharge(ctx context.Context, prices []SubscriptionPrice, document Document) (bool, error)
	SubscriptionCorrections(context.Context) ([]Correction, error)
	OriginalUsage(context.Context, int64) ([]domain.UsageShare, error)
	CreateAdjustment(context.Context, string, Document) (bool, error)
	UsageRatingCandidates(context.Context, RatingScan, int64, int32) ([]UsageRatingCandidate, error)
	CreateUsageRating(context.Context, UsageRating) (bool, error)
	APIKeyPeriods(context.Context, time.Time) ([]APIKeyPeriod, error)
	APIKeyUsage(context.Context, APIKeyPeriod) ([]domain.RatedShare, []int64, error)
	APIKeyCurrent(context.Context, APIKeyPeriod) (APIKeyCurrent, error)
	CreateAPIKeyDocument(context.Context, string, int64, Document, []int64) (bool, error)
	ConfirmAPIKeyRatings(context.Context, APIKeyConfirmation) (bool, error)
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
// 单个账期失败（含与已出账账期重叠）只计入 Failed 后继续；读取价格或已出账账期失败、
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
	charged := make(map[int64][]ChargePeriod)
	for _, period := range periods {
		existing[newPeriodKey(period.CredentialID, period.PeriodStart, period.PeriodEnd)] = struct{}{}
		charged[period.CredentialID] = append(charged[period.CredentialID], period)
	}

	groups := groupSubscriptionPrices(prices)
	periodCount := 0
	for _, group := range groups {
		// abort 记录需要中止整轮的全局错误；回调返回 false 只结束当前凭据。
		var abort error
		walkErr := walkSubscriptionPeriods(group, cutoff, 0, func(period subscriptionPeriod) bool {
			price := period.price()
			periodStart, periodEnd := period.Start, period.End
			if periodEnd.After(cutoff) {
				return false
			}
			key := newPeriodKey(price.CredentialID, periodStart, periodEnd)
			if _, found := existing[key]; found {
				summary.Skipped++
				return true
			}
			periodCount++
			if periodCount > maxPeriodsPerScan {
				abort = errors.New("billing settlement scan exceeded period limit")
				return false
			}
			if overlap := overlappingCharge(charged[price.CredentialID], periodStart, periodEnd); overlap != nil {
				// 价格或凭据生效时间被补录、修改后锚点可能漂移；重叠账期不能再次出账，只记录失败等待人工处理。
				summary.recordFailure("subscription credential=%d period=%s/%s overlaps charged period %s/%s",
					price.CredentialID, formatPeriodTime(periodStart), formatPeriodTime(periodEnd),
					formatPeriodTime(overlap.PeriodStart), formatPeriodTime(overlap.PeriodEnd))
				return true
			}
			created, chargeErr := s.settleSubscriptionPeriod(ctx, period.History, periodStart, periodEnd)
			if chargeErr != nil {
				if ctx.Err() != nil {
					abort = ctx.Err()
					return false
				}
				if errors.Is(chargeErr, errBillingID) {
					abort = chargeErr
					return false
				}
				if errors.Is(chargeErr, ErrSubscriptionPricesChanged) {
					// 本轮读取的价格已过期，后续账期同样基于过期价格推算；跳过该凭据，下一轮按新价格重新计算。
					summary.Skipped++
					return false
				}
				summary.recordFailure("subscription credential=%d period=%s/%s: %s", price.CredentialID,
					formatPeriodTime(periodStart), formatPeriodTime(periodEnd), SafeErrorText(chargeErr))
				return true
			}
			if created {
				summary.Created++
			} else {
				summary.Skipped++
			}
			existing[key] = struct{}{}
			charged[price.CredentialID] = append(charged[price.CredentialID],
				ChargePeriod{CredentialID: price.CredentialID, PeriodStart: periodStart, PeriodEnd: periodEnd})
			return true
		})
		if abort != nil {
			return summary, abort
		}
		if walkErr != nil {
			summary.recordFailure("subscription credential=%d %v", group[0].CredentialID, walkErr)
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

// settleSubscriptionPeriod 按 history 最后一条价格为账期出账；history 随单据交给存储层校验。
func (s *Service) settleSubscriptionPeriod(ctx context.Context, history []SubscriptionPrice, periodStart, periodEnd time.Time) (bool, error) {
	price := &history[len(history)-1]
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
	return s.store.CreateCharge(ctx, history, document)
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

// overlappingCharge 返回与 [start, end) 部分重叠（但不完全相同）的已出账账期。
func overlappingCharge(periods []ChargePeriod, start, end time.Time) *ChargePeriod {
	for index := range periods {
		period := &periods[index]
		if period.PeriodStart.Equal(start) && period.PeriodEnd.Equal(end) {
			continue
		}
		if period.PeriodStart.Before(end) && period.PeriodEnd.After(start) {
			return period
		}
	}
	return nil
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

// pricesUntil 返回按生效时间排序的 prices 中不晚于 at 生效的前缀，最后一条即 at 时的有效价格。
func pricesUntil(prices []SubscriptionPrice, at time.Time) []SubscriptionPrice {
	count := 0
	for count < len(prices) && !prices[count].EffectiveAt.After(at) {
		count++
	}
	return prices[:count:count]
}

// subscriptionPeriod 是按出账规则推算出的一个订阅账期。History 为账期起点及之前生效的价格历史，
// 最后一条即该账期计费采用的价格。
type subscriptionPeriod struct {
	Start, End time.Time
	History    []SubscriptionPrice
}

func (p subscriptionPeriod) price() *SubscriptionPrice { return &p.History[len(p.History)-1] }

var errSubscriptionPeriodLimit = errors.New("subscription period scan exceeded period limit")

// walkSubscriptionPeriods 从凭据锚点起按出账规则依次推算 group 的账期，对起点不晚于 until 的每个账期
// 调用 visit；visit 返回 false、凭据结束或账期起点没有有效价格时停止。出账与当前账期查询共用这一规则，
// 保证展示的账期与实际出账账期一致。maxPeriods 大于 0 时限制推算的账期数，超出返回 errSubscriptionPeriodLimit；
// 出账传 0，由调用方只对需要新出账的账期计数，已出账的历史账期不受限。无法推算账期结束时间时返回带价格 ID 的错误。
func walkSubscriptionPeriods(group []SubscriptionPrice, until time.Time, maxPeriods int, visit func(subscriptionPeriod) bool) error {
	if len(group) == 0 {
		return nil
	}
	until = until.UTC()
	periodStart := subscriptionAnchor(group)
	anchorDay := periodStart.Day()
	for count := 0; !periodStart.After(until); count++ {
		if maxPeriods > 0 && count >= maxPeriods {
			return errSubscriptionPeriodLimit
		}
		if group[0].CredentialEndAt != nil && !periodStart.Before(group[0].CredentialEndAt.UTC()) {
			return nil
		}
		history := pricesUntil(group, periodStart)
		if len(history) == 0 {
			return nil
		}
		price := &history[len(history)-1]
		periodEnd, err := addBillingPeriod(periodStart, price.BillingPeriod, anchorDay)
		if err != nil {
			return fmt.Errorf("price=%d: %w", price.ID, err)
		}
		if !visit(subscriptionPeriod{Start: periodStart, End: periodEnd, History: history}) {
			return nil
		}
		periodStart = periodEnd
	}
	return nil
}

// subscriptionPeriodAt 推算 at 所在的订阅账期，返回账期起止时间及该账期计费采用的价格；
// at 不在任何可计费账期内时 price 为 nil。
func subscriptionPeriodAt(group []SubscriptionPrice, at time.Time) (time.Time, time.Time, *SubscriptionPrice, error) {
	at = at.UTC()
	var current *subscriptionPeriod
	err := walkSubscriptionPeriods(group, at, maxPeriodsPerScan+1, func(period subscriptionPeriod) bool {
		if period.End.After(at) {
			current = &period
			return false
		}
		return true
	})
	if err != nil || current == nil {
		return time.Time{}, time.Time{}, nil, err
	}
	return current.Start, current.End, current.price(), nil
}

// subscriptionAnchor 返回凭据首个账期的起点：最早价格生效时间与凭据生效时间中的较晚者。
func subscriptionAnchor(group []SubscriptionPrice) time.Time {
	periodStart := group[0].EffectiveAt.UTC()
	if group[0].CredentialEffectiveAt.After(periodStart) {
		periodStart = group[0].CredentialEffectiveAt.UTC()
	}
	return periodStart
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
