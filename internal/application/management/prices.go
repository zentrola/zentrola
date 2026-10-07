package management

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/operation"
	"github.com/zentrola/zentrola/internal/domain/shared"
)

// 价格使用十进制字符串，避免管理 API 经由浮点数丢失精度。
var priceDecimal = regexp.MustCompile(`^(0|[1-9][0-9]{0,11})(\.[0-9]{1,8})?$`)

type ModelPrice struct {
	ID                   int64     `json:"id,string"`
	ProviderCredentialID int64     `json:"providerCredentialId,string"`
	ProviderModelID      int64     `json:"providerModelId,string"`
	Currency             string    `json:"currency"`
	InputPrice           string    `json:"inputPrice"`
	OutputPrice          string    `json:"outputPrice"`
	CachedInputPrice     string    `json:"cachedInputPrice"`
	EffectiveAt          time.Time `json:"effectiveAt"`
	CreatedAt            time.Time `json:"createdAt"`
}

type SubscriptionPrice struct {
	ID                   int64     `json:"id,string"`
	ProviderCredentialID int64     `json:"providerCredentialId,string"`
	Currency             string    `json:"currency"`
	PeriodAmount         string    `json:"periodAmount"`
	BillingPeriod        string    `json:"billingPeriod"`
	EffectiveAt          time.Time `json:"effectiveAt"`
	CreatedAt            time.Time `json:"createdAt"`
}

type CredentialPrices struct {
	ModelPrices        []ModelPrice        `json:"modelPrices"`
	SubscriptionPrices []SubscriptionPrice `json:"subscriptionPrices"`
	SubscriptionPrice  *SubscriptionPrice  `json:"subscriptionPrice"`
}

type ModelPriceInput struct {
	ProviderModelID  int64
	Currency         string
	InputPrice       string
	OutputPrice      string
	CachedInputPrice string
	EffectiveAt      time.Time
}

type SubscriptionPriceInput struct {
	Currency      string
	PeriodAmount  string
	BillingPeriod string
	EffectiveAt   time.Time
}

type PriceReadSession interface {
	Resource(context.Context, int64) (ResourceRecord, error)
	ProviderMappings(context.Context, int64) ([]ProviderMapping, error)
	ModelPrices(context.Context, int64) ([]ModelPrice, error)
	SubscriptionPrices(context.Context, int64) ([]SubscriptionPrice, error)
}

type PriceWriteSession interface {
	PriceReadSession
	InsertModelPrice(context.Context, ModelPrice, string) error
	UpdateModelPrice(context.Context, ModelPrice) (bool, error)
	DeleteModelPrice(context.Context, int64, int64, int64) (bool, error)
	InsertSubscriptionPrice(context.Context, SubscriptionPrice, string) error
	UpdateSubscriptionPrice(context.Context, SubscriptionPrice) (bool, error)
	DeleteSubscriptionPrice(context.Context, int64, int64) (bool, error)
	SubscriptionPriceReferenced(context.Context, int64) (bool, error)
	Audit(context.Context, Audit, appsec.RequestMeta) error
}

func findModelPrice(prices []ModelPrice, credentialID, modelID, priceID int64) *ModelPrice {
	for index := range prices {
		price := &prices[index]
		if price.ID == priceID && price.ProviderCredentialID == credentialID && price.ProviderModelID == modelID {
			return price
		}
	}
	return nil
}

func sameModelPrice(left ModelPrice, input ModelPriceInput) bool {
	return left.Currency == input.Currency && left.InputPrice == input.InputPrice &&
		left.OutputPrice == input.OutputPrice && left.CachedInputPrice == input.CachedInputPrice &&
		left.EffectiveAt.Equal(input.EffectiveAt)
}

func findSubscriptionPrice(prices []SubscriptionPrice, credentialID, priceID int64) *SubscriptionPrice {
	for index := range prices {
		price := &prices[index]
		if price.ID == priceID && price.ProviderCredentialID == credentialID {
			return price
		}
	}
	return nil
}

func sameSubscriptionPrice(left SubscriptionPrice, input SubscriptionPriceInput) bool {
	return left.Currency == input.Currency && left.PeriodAmount == input.PeriodAmount &&
		left.BillingPeriod == input.BillingPeriod && left.EffectiveAt.Equal(input.EffectiveAt)
}

type PriceStore interface {
	ReadPrice(context.Context, admin.Identity, func(PriceReadSession) error) error
	WritePrice(context.Context, admin.Identity, func(PriceWriteSession) error) error
}

type PriceService struct {
	store PriceStore
	ids   shared.IDGenerator
	now   func() time.Time
}

func validPriceCurrency(currency string) bool { return currency == "CNY" || currency == "USD" }

func normalizePriceDecimal(value string) string {
	if strings.Contains(value, ".") {
		value = strings.TrimRight(strings.TrimRight(value, "0"), ".")
	}
	return value
}

func (input ModelPriceInput) valid() bool {
	return input.ProviderModelID > 0 && validPriceCurrency(input.Currency) &&
		priceDecimal.MatchString(input.InputPrice) && priceDecimal.MatchString(input.OutputPrice) &&
		priceDecimal.MatchString(input.CachedInputPrice) && validPriceDate(input.EffectiveAt)
}

func validPriceDate(value time.Time) bool {
	return !value.IsZero() && value.Location() == time.UTC &&
		value.Hour() == 0 && value.Minute() == 0 && value.Second() == 0 && value.Nanosecond() == 0
}

func (input SubscriptionPriceInput) valid() bool {
	return validPriceCurrency(input.Currency) && priceDecimal.MatchString(input.PeriodAmount) &&
		(input.BillingPeriod == "MONTH" || input.BillingPeriod == "YEAR") &&
		validPriceDate(input.EffectiveAt)
}

func (s *PriceService) CredentialPrices(ctx context.Context, actor admin.Identity, credentialID int64) (CredentialPrices, error) {
	result := CredentialPrices{ModelPrices: []ModelPrice{}, SubscriptionPrices: []SubscriptionPrice{}}
	if credentialID <= 0 {
		return result, appsec.ErrInvalidArgument
	}
	if s == nil || dependencyMissing(s.store) {
		return result, appsec.ErrUnavailable
	}
	err := s.store.ReadPrice(ctx, actor, func(reader PriceReadSession) error {
		resource, err := reader.Resource(ctx, credentialID)
		if err != nil {
			return err
		}
		if resource.AuthType == "SUBSCRIPTION" {
			result.SubscriptionPrices, err = reader.SubscriptionPrices(ctx, credentialID)
			if err == nil {
				now := businessTime(s.now)
				for index := range result.SubscriptionPrices {
					if !result.SubscriptionPrices[index].EffectiveAt.After(now) {
						result.SubscriptionPrice = &result.SubscriptionPrices[index]
						break
					}
				}
			}
		} else {
			result.ModelPrices, err = reader.ModelPrices(ctx, credentialID)
		}
		return err
	})
	return result, err
}

func (s *PriceService) SaveModelPrice(ctx context.Context, actor admin.Identity, credentialID int64, input ModelPriceInput, meta appsec.RequestMeta) (ModelPrice, error) {
	var saved ModelPrice
	if credentialID <= 0 || !input.valid() {
		return saved, appsec.ErrInvalidArgument
	}
	input.InputPrice = normalizePriceDecimal(input.InputPrice)
	input.OutputPrice = normalizePriceDecimal(input.OutputPrice)
	input.CachedInputPrice = normalizePriceDecimal(input.CachedInputPrice)
	if s == nil || dependencyMissing(s.store) {
		return saved, appsec.ErrUnavailable
	}
	err := s.store.WritePrice(ctx, actor, func(writer PriceWriteSession) error {
		resource, err := writer.Resource(ctx, credentialID)
		if err != nil {
			return err
		}
		if resource.AuthType != "API_KEY" {
			return appsec.ErrInvalidArgument
		}
		mappings, err := writer.ProviderMappings(ctx, resource.ProviderID)
		if err != nil {
			return err
		}
		found := false
		for _, mapping := range mappings {
			if mapping.ID == input.ProviderModelID {
				found = true
				break
			}
		}
		if !found {
			return appsec.ErrNotFound
		}
		previous, err := writer.ModelPrices(ctx, credentialID)
		if err != nil {
			return err
		}
		var before *ModelPrice
		for index := range previous {
			price := &previous[index]
			if price.ProviderModelID != input.ProviderModelID {
				continue
			}
			if price.EffectiveAt.Equal(input.EffectiveAt) {
				if price.Currency == input.Currency && price.InputPrice == input.InputPrice &&
					price.OutputPrice == input.OutputPrice && price.CachedInputPrice == input.CachedInputPrice {
					saved = *price
					return nil
				}
				return ErrConflict
			}
			if before == nil && price.EffectiveAt.Before(input.EffectiveAt) {
				before = price
			}
		}
		id, err := nextID(ctx, s.ids)
		if err != nil {
			return err
		}
		at := businessTime(s.now)
		saved = ModelPrice{ID: id, ProviderCredentialID: credentialID, ProviderModelID: input.ProviderModelID,
			Currency: input.Currency, InputPrice: input.InputPrice, OutputPrice: input.OutputPrice,
			CachedInputPrice: input.CachedInputPrice, EffectiveAt: input.EffectiveAt.UTC(), CreatedAt: at}
		if err := writer.InsertModelPrice(ctx, saved, "admin:"+strconv.FormatInt(actor.ID, 10)); err != nil {
			return err
		}
		var beforeSnapshot any
		if before != nil {
			beforeSnapshot = *before
		}
		return writer.Audit(ctx, Audit{Event: operation.ResourcePriceUpdate, Target: "RESOURCE", ID: credentialID,
			Name: resource.Name, Before: beforeSnapshot, After: saved}, meta)
	})
	return saved, err
}

func (s *PriceService) UpdateModelPrice(ctx context.Context, actor admin.Identity, credentialID, priceID int64, input ModelPriceInput, meta appsec.RequestMeta) (ModelPrice, error) {
	var saved ModelPrice
	if credentialID <= 0 || priceID <= 0 || !input.valid() {
		return saved, appsec.ErrInvalidArgument
	}
	input.InputPrice = normalizePriceDecimal(input.InputPrice)
	input.OutputPrice = normalizePriceDecimal(input.OutputPrice)
	input.CachedInputPrice = normalizePriceDecimal(input.CachedInputPrice)
	if s == nil || dependencyMissing(s.store) {
		return saved, appsec.ErrUnavailable
	}
	err := s.store.WritePrice(ctx, actor, func(writer PriceWriteSession) error {
		resource, err := writer.Resource(ctx, credentialID)
		if err != nil {
			return err
		}
		if resource.AuthType != "API_KEY" {
			return appsec.ErrInvalidArgument
		}
		prices, err := writer.ModelPrices(ctx, credentialID)
		if err != nil {
			return err
		}
		before := findModelPrice(prices, credentialID, input.ProviderModelID, priceID)
		if before == nil {
			return appsec.ErrNotFound
		}
		if sameModelPrice(*before, input) {
			saved = *before
			return nil
		}
		for _, price := range prices {
			if price.ID != priceID && price.ProviderModelID == input.ProviderModelID && price.EffectiveAt.Equal(input.EffectiveAt) {
				return ErrConflict
			}
		}
		saved = *before
		saved.Currency = input.Currency
		saved.InputPrice = input.InputPrice
		saved.OutputPrice = input.OutputPrice
		saved.CachedInputPrice = input.CachedInputPrice
		saved.EffectiveAt = input.EffectiveAt.UTC()
		updated, err := writer.UpdateModelPrice(ctx, saved)
		if err != nil {
			return err
		}
		if !updated {
			return appsec.ErrNotFound
		}
		return writer.Audit(ctx, Audit{Event: operation.ResourcePriceUpdate, Target: "RESOURCE", ID: credentialID,
			Name: resource.Name, Before: *before, After: saved}, meta)
	})
	return saved, err
}

func (s *PriceService) DeleteModelPrice(ctx context.Context, actor admin.Identity, credentialID, modelID, priceID int64, meta appsec.RequestMeta) error {
	if credentialID <= 0 || modelID <= 0 || priceID <= 0 {
		return appsec.ErrInvalidArgument
	}
	if s == nil || dependencyMissing(s.store) {
		return appsec.ErrUnavailable
	}
	return s.store.WritePrice(ctx, actor, func(writer PriceWriteSession) error {
		resource, err := writer.Resource(ctx, credentialID)
		if err != nil {
			return err
		}
		if resource.AuthType != "API_KEY" {
			return appsec.ErrInvalidArgument
		}
		prices, err := writer.ModelPrices(ctx, credentialID)
		if err != nil {
			return err
		}
		before := findModelPrice(prices, credentialID, modelID, priceID)
		if before == nil {
			return appsec.ErrNotFound
		}
		deleted, err := writer.DeleteModelPrice(ctx, credentialID, modelID, priceID)
		if err != nil {
			return err
		}
		if !deleted {
			return appsec.ErrNotFound
		}
		return writer.Audit(ctx, Audit{Event: operation.ResourcePriceUpdate, Target: "RESOURCE", ID: credentialID,
			Name: resource.Name, Before: *before, After: nil}, meta)
	})
}

func (s *PriceService) SaveSubscriptionPrice(ctx context.Context, actor admin.Identity, credentialID int64, input SubscriptionPriceInput, meta appsec.RequestMeta) (SubscriptionPrice, error) {
	var saved SubscriptionPrice
	if credentialID <= 0 || !input.valid() {
		return saved, appsec.ErrInvalidArgument
	}
	input.PeriodAmount = normalizePriceDecimal(input.PeriodAmount)
	if s == nil || dependencyMissing(s.store) {
		return saved, appsec.ErrUnavailable
	}
	err := s.store.WritePrice(ctx, actor, func(writer PriceWriteSession) error {
		resource, err := writer.Resource(ctx, credentialID)
		if err != nil {
			return err
		}
		if resource.AuthType != "SUBSCRIPTION" {
			return appsec.ErrInvalidArgument
		}
		prices, err := writer.SubscriptionPrices(ctx, credentialID)
		if err != nil {
			return err
		}
		var before *SubscriptionPrice
		for index := range prices {
			price := &prices[index]
			if price.EffectiveAt.Equal(input.EffectiveAt) {
				if sameSubscriptionPrice(*price, input) {
					saved = *price
					return nil
				}
				return ErrConflict
			}
			if before == nil && price.EffectiveAt.Before(input.EffectiveAt) {
				before = price
			}
		}
		id, err := nextID(ctx, s.ids)
		if err != nil {
			return err
		}
		at := businessTime(s.now)
		saved = SubscriptionPrice{ID: id, ProviderCredentialID: credentialID, Currency: input.Currency,
			PeriodAmount: input.PeriodAmount, BillingPeriod: input.BillingPeriod,
			EffectiveAt: input.EffectiveAt.UTC(), CreatedAt: at}
		if err := writer.InsertSubscriptionPrice(ctx, saved, "admin:"+strconv.FormatInt(actor.ID, 10)); err != nil {
			return err
		}
		var beforeSnapshot any
		if before != nil {
			beforeSnapshot = *before
		}
		return writer.Audit(ctx, Audit{Event: operation.ResourcePriceUpdate, Target: "RESOURCE", ID: credentialID,
			Name: resource.Name, Before: beforeSnapshot, After: saved}, meta)
	})
	return saved, err
}

func (s *PriceService) UpdateSubscriptionPrice(ctx context.Context, actor admin.Identity, credentialID, priceID int64, input SubscriptionPriceInput, meta appsec.RequestMeta) (SubscriptionPrice, error) {
	var saved SubscriptionPrice
	if credentialID <= 0 || priceID <= 0 || !input.valid() {
		return saved, appsec.ErrInvalidArgument
	}
	input.PeriodAmount = normalizePriceDecimal(input.PeriodAmount)
	if s == nil || dependencyMissing(s.store) {
		return saved, appsec.ErrUnavailable
	}
	err := s.store.WritePrice(ctx, actor, func(writer PriceWriteSession) error {
		resource, err := writer.Resource(ctx, credentialID)
		if err != nil {
			return err
		}
		if resource.AuthType != "SUBSCRIPTION" {
			return appsec.ErrInvalidArgument
		}
		prices, err := writer.SubscriptionPrices(ctx, credentialID)
		if err != nil {
			return err
		}
		before := findSubscriptionPrice(prices, credentialID, priceID)
		if before == nil {
			return appsec.ErrNotFound
		}
		if sameSubscriptionPrice(*before, input) {
			saved = *before
			return nil
		}
		if before.Currency != input.Currency || before.BillingPeriod != input.BillingPeriod || !before.EffectiveAt.Equal(input.EffectiveAt) {
			referenced, inspectErr := writer.SubscriptionPriceReferenced(ctx, priceID)
			if inspectErr != nil {
				return inspectErr
			}
			if referenced {
				return ErrConflict
			}
		}
		for _, price := range prices {
			if price.ID != priceID && price.EffectiveAt.Equal(input.EffectiveAt) {
				return ErrConflict
			}
		}
		saved = *before
		saved.Currency = input.Currency
		saved.PeriodAmount = input.PeriodAmount
		saved.BillingPeriod = input.BillingPeriod
		saved.EffectiveAt = input.EffectiveAt.UTC()
		updated, err := writer.UpdateSubscriptionPrice(ctx, saved)
		if err != nil {
			return err
		}
		if !updated {
			return appsec.ErrNotFound
		}
		return writer.Audit(ctx, Audit{Event: operation.ResourcePriceUpdate, Target: "RESOURCE", ID: credentialID,
			Name: resource.Name, Before: *before, After: saved}, meta)
	})
	return saved, err
}

func (s *PriceService) DeleteSubscriptionPrice(ctx context.Context, actor admin.Identity, credentialID, priceID int64, meta appsec.RequestMeta) error {
	if credentialID <= 0 || priceID <= 0 {
		return appsec.ErrInvalidArgument
	}
	if s == nil || dependencyMissing(s.store) {
		return appsec.ErrUnavailable
	}
	return s.store.WritePrice(ctx, actor, func(writer PriceWriteSession) error {
		resource, err := writer.Resource(ctx, credentialID)
		if err != nil {
			return err
		}
		if resource.AuthType != "SUBSCRIPTION" {
			return appsec.ErrInvalidArgument
		}
		prices, err := writer.SubscriptionPrices(ctx, credentialID)
		if err != nil {
			return err
		}
		before := findSubscriptionPrice(prices, credentialID, priceID)
		if before == nil {
			return appsec.ErrNotFound
		}
		referenced, inspectErr := writer.SubscriptionPriceReferenced(ctx, priceID)
		if inspectErr != nil {
			return inspectErr
		}
		if referenced {
			return ErrConflict
		}
		deleted, err := writer.DeleteSubscriptionPrice(ctx, credentialID, priceID)
		if err != nil {
			return err
		}
		if !deleted {
			return appsec.ErrNotFound
		}
		return writer.Audit(ctx, Audit{Event: operation.ResourcePriceUpdate, Target: "RESOURCE", ID: credentialID,
			Name: resource.Name, Before: *before, After: nil}, meta)
	})
}
