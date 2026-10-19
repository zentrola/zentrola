package management

import (
	"context"
	"errors"
	"testing"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
)

type priceIDs struct{ next int64 }

func (g *priceIDs) NextID(context.Context) (int64, error) { g.next++; return g.next, nil }

type priceStoreStub struct {
	resource    ResourceRecord
	mappings    []ProviderMapping
	models      []ModelPrice
	subs        []SubscriptionPrice
	audits      int
	auditBefore []any
	writes      int
	referenced  bool
	// modelReferenced 与 overrides 模拟模型价格的核算引用与历史覆盖检查结果。
	modelReferenced bool
	overrides       bool
	chargedUntil    *time.Time
}

func (s *priceStoreStub) ReadPrice(_ context.Context, _ admin.Identity, fn func(PriceReadSession) error) error {
	return fn(s)
}
func (s *priceStoreStub) WritePrice(_ context.Context, _ admin.Identity, fn func(PriceWriteSession) error) error {
	s.writes++
	return fn(s)
}
func (s *priceStoreStub) Resource(_ context.Context, id int64) (ResourceRecord, error) {
	if s.resource.ID != id {
		return ResourceRecord{}, appsec.ErrNotFound
	}
	return s.resource, nil
}
func (s *priceStoreStub) ProviderMappings(context.Context, int64) ([]ProviderMapping, error) {
	return s.mappings, nil
}
func (s *priceStoreStub) ModelPrices(_ context.Context, _ int64) ([]ModelPrice, error) {
	return append([]ModelPrice(nil), s.models...), nil
}
func (s *priceStoreStub) SubscriptionPrices(_ context.Context, _ int64) ([]SubscriptionPrice, error) {
	return append([]SubscriptionPrice(nil), s.subs...), nil
}
func (s *priceStoreStub) InsertModelPrice(_ context.Context, price ModelPrice, _ string) error {
	s.models = append([]ModelPrice{price}, s.models...)
	return nil
}
func (s *priceStoreStub) UpdateModelPrice(_ context.Context, price ModelPrice) (bool, error) {
	for index := range s.models {
		if s.models[index].ID == price.ID {
			s.models[index] = price
			return true, nil
		}
	}
	return false, nil
}
func (s *priceStoreStub) DeleteModelPrice(_ context.Context, _, _, priceID int64) (bool, error) {
	for index := range s.models {
		if s.models[index].ID == priceID {
			s.models = append(s.models[:index], s.models[index+1:]...)
			return true, nil
		}
	}
	return false, nil
}
func (s *priceStoreStub) InsertSubscriptionPrice(_ context.Context, price SubscriptionPrice, _ string) error {
	s.subs = append([]SubscriptionPrice{price}, s.subs...)
	return nil
}
func (s *priceStoreStub) UpdateSubscriptionPrice(_ context.Context, price SubscriptionPrice) (bool, error) {
	for index := range s.subs {
		if s.subs[index].ID == price.ID {
			s.subs[index] = price
			return true, nil
		}
	}
	return false, nil
}
func (s *priceStoreStub) DeleteSubscriptionPrice(_ context.Context, _, priceID int64) (bool, error) {
	for index := range s.subs {
		if s.subs[index].ID == priceID {
			s.subs = append(s.subs[:index], s.subs[index+1:]...)
			return true, nil
		}
	}
	return false, nil
}
func (s *priceStoreStub) Audit(_ context.Context, audit Audit, _ appsec.RequestMeta) error {
	s.audits++
	s.auditBefore = append(s.auditBefore, audit.Before)
	return nil
}
func (s *priceStoreStub) SubscriptionPriceReferenced(context.Context, int64) (bool, error) {
	return s.referenced, nil
}
func (s *priceStoreStub) SubscriptionChargedUntil(context.Context, int64) (*time.Time, error) {
	return s.chargedUntil, nil
}
func (s *priceStoreStub) ModelPriceReferenced(context.Context, int64) (bool, error) {
	return s.modelReferenced, nil
}
func (s *priceStoreStub) ModelPriceOverridesRatedUsage(context.Context, ModelPrice) (bool, error) {
	return s.overrides, nil
}

func TestPriceInputValidationBeforeStore(t *testing.T) {
	store := &priceStoreStub{}
	service := PriceService{store: store}
	effectiveAt := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for _, input := range []ModelPriceInput{
		{ProviderModelID: 2, Currency: "EUR", InputPrice: "1", OutputPrice: "1", CachedInputPrice: "1", EffectiveAt: effectiveAt},
		{ProviderModelID: 2, Currency: "USD", InputPrice: "-1", OutputPrice: "1", CachedInputPrice: "1", EffectiveAt: effectiveAt},
		{ProviderModelID: 2, Currency: "USD", InputPrice: "1.123456789", OutputPrice: "1", CachedInputPrice: "1", EffectiveAt: effectiveAt},
		{ProviderModelID: 2, Currency: "USD", InputPrice: "1", OutputPrice: "1", CachedInputPrice: "1"},
		{ProviderModelID: 2, Currency: "USD", InputPrice: "1", OutputPrice: "1", CachedInputPrice: "1", EffectiveAt: atNineUTC(effectiveAt)},
	} {
		if _, err := service.SaveModelPrice(context.Background(), admin.Identity{}, 1, input, appsec.RequestMeta{}); !errors.Is(err, appsec.ErrInvalidArgument) {
			t.Fatalf("SaveModelPrice(%+v) error = %v", input, err)
		}
	}
	if store.writes != 0 {
		t.Fatal("invalid price reached store")
	}
}

func TestSaveModelPriceCreatesVersionsAndNoopsWhenUnchanged(t *testing.T) {
	at := time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)
	store := &priceStoreStub{
		resource: ResourceRecord{Resource: Resource{ID: 1, ProviderID: 3, Name: "key", AuthType: "API_KEY"}},
		mappings: []ProviderMapping{{ID: 2, ProviderID: 3}},
	}
	ids := &priceIDs{}
	service := PriceService{store: store, ids: ids, now: func() time.Time { return at }}
	input := ModelPriceInput{ProviderModelID: 2, Currency: "USD", InputPrice: "1", OutputPrice: "2", CachedInputPrice: "0.5",
		EffectiveAt: time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)}
	first, err := service.SaveModelPrice(context.Background(), admin.Identity{ID: 7}, 1, input, appsec.RequestMeta{})
	if err != nil || first.ID != 1 || store.audits != 1 {
		t.Fatalf("first = %+v, audit = %d, err = %v", first, store.audits, err)
	}
	if store.auditBefore[0] != nil {
		t.Fatalf("first price audit before = %#v; want nil", store.auditBefore[0])
	}
	same, err := service.SaveModelPrice(context.Background(), admin.Identity{ID: 7}, 1, input, appsec.RequestMeta{})
	if err != nil || same.ID != first.ID || store.audits != 1 {
		t.Fatalf("same = %+v, audit = %d, err = %v", same, store.audits, err)
	}
	input.OutputPrice = "3"
	if _, err := service.SaveModelPrice(context.Background(), admin.Identity{ID: 7}, 1, input, appsec.RequestMeta{}); !errors.Is(err, ErrConflict) {
		t.Fatalf("same effective date error = %v; want conflict", err)
	}
	input.EffectiveAt = time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	changed, err := service.SaveModelPrice(context.Background(), admin.Identity{ID: 7}, 1, input, appsec.RequestMeta{})
	if err != nil || changed.ID != 2 || store.audits != 2 || !changed.EffectiveAt.After(first.EffectiveAt) {
		t.Fatalf("changed = %+v, audit = %d, err = %v", changed, store.audits, err)
	}
	if store.auditBefore[1] == nil {
		t.Fatal("updated price audit has no before snapshot")
	}
	result, err := service.CredentialPrices(context.Background(), admin.Identity{ID: 7}, 1)
	if err != nil || len(result.ModelPrices) != 2 || result.ModelPrices[0].ID != changed.ID || result.ModelPrices[1].ID != first.ID {
		t.Fatalf("price history = %+v, err = %v", result.ModelPrices, err)
	}
}

func TestSavePriceRejectsWrongCredentialTypeAndModel(t *testing.T) {
	store := &priceStoreStub{resource: ResourceRecord{Resource: Resource{ID: 1, ProviderID: 3, AuthType: "SUBSCRIPTION"}}}
	service := PriceService{store: store}
	input := ModelPriceInput{ProviderModelID: 2, Currency: "USD", InputPrice: "1", OutputPrice: "2", CachedInputPrice: "0.5",
		EffectiveAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	if _, err := service.SaveModelPrice(context.Background(), admin.Identity{}, 1, input, appsec.RequestMeta{}); !errors.Is(err, appsec.ErrInvalidArgument) {
		t.Fatalf("wrong auth type error = %v", err)
	}
	store.resource.AuthType = "API_KEY"
	if _, err := service.SaveModelPrice(context.Background(), admin.Identity{}, 1, input, appsec.RequestMeta{}); !errors.Is(err, appsec.ErrNotFound) {
		t.Fatalf("missing mapping error = %v", err)
	}
}

func TestUpdateAndDeleteModelPrice(t *testing.T) {
	effectiveAt := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	store := &priceStoreStub{
		resource: ResourceRecord{Resource: Resource{ID: 1, ProviderID: 3, Name: "key", AuthType: "API_KEY"}},
		models: []ModelPrice{{ID: 9, ProviderCredentialID: 1, ProviderModelID: 2, Currency: "USD",
			InputPrice: "1", OutputPrice: "2", CachedInputPrice: "0.5", EffectiveAt: effectiveAt, CreatedAt: effectiveAt}},
	}
	service := PriceService{store: store}
	input := ModelPriceInput{ProviderModelID: 2, Currency: "CNY", InputPrice: "3", OutputPrice: "4",
		CachedInputPrice: "1", EffectiveAt: effectiveAt.AddDate(0, 0, 1)}
	updated, err := service.UpdateModelPrice(context.Background(), admin.Identity{ID: 7}, 1, 9, input, appsec.RequestMeta{})
	if err != nil || updated.Currency != "CNY" || store.audits != 1 {
		t.Fatalf("updated = %+v, audits = %d, err = %v", updated, store.audits, err)
	}
	if err := service.DeleteModelPrice(context.Background(), admin.Identity{ID: 7}, 1, 2, 9, appsec.RequestMeta{}); err != nil || len(store.models) != 0 || store.audits != 2 {
		t.Fatalf("models = %+v, audits = %d, err = %v", store.models, store.audits, err)
	}
}

func atNineUTC(value time.Time) time.Time { return value.Add(9 * time.Hour) }

func TestSaveSubscriptionPrice(t *testing.T) {
	at := time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)
	store := &priceStoreStub{resource: ResourceRecord{Resource: Resource{ID: 1, AuthType: "SUBSCRIPTION"}}}
	now := at
	service := PriceService{store: store, ids: &priceIDs{}, now: func() time.Time { return now }}
	effectiveAt := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	input := SubscriptionPriceInput{Currency: "CNY", PeriodAmount: "100", BillingPeriod: "MONTH", EffectiveAt: effectiveAt}
	saved, err := service.SaveSubscriptionPrice(context.Background(), admin.Identity{ID: 7}, 1, input, appsec.RequestMeta{})
	if err != nil || saved.PeriodAmount != "100" || store.audits != 1 {
		t.Fatalf("saved = %+v, audit = %d, err = %v", saved, store.audits, err)
	}
	if store.auditBefore[0] != nil {
		t.Fatalf("first subscription audit before = %#v; want nil", store.auditBefore[0])
	}
	now = now.Add(time.Second)
	input.PeriodAmount = "120"
	if _, err := service.SaveSubscriptionPrice(context.Background(), admin.Identity{ID: 7}, 1, input, appsec.RequestMeta{}); !errors.Is(err, ErrConflict) {
		t.Fatalf("same subscription effective date error = %v; want conflict", err)
	}
	input.EffectiveAt = effectiveAt.AddDate(0, 0, 1)
	updated, err := service.SaveSubscriptionPrice(context.Background(), admin.Identity{ID: 7}, 1, input, appsec.RequestMeta{})
	if err != nil || updated.ID == saved.ID || store.audits != 2 {
		t.Fatalf("updated = %+v, audit = %d, err = %v", updated, store.audits, err)
	}
	if store.auditBefore[1] == nil {
		t.Fatal("updated subscription audit has no before snapshot")
	}
	result, err := service.CredentialPrices(context.Background(), admin.Identity{ID: 7}, 1)
	if err != nil || len(result.SubscriptionPrices) != 2 || result.SubscriptionPrice == nil || result.SubscriptionPrice.ID != updated.ID {
		t.Fatalf("prices = %+v, err = %v", result, err)
	}
	input.EffectiveAt = saved.EffectiveAt
	if _, err := service.UpdateSubscriptionPrice(context.Background(), admin.Identity{ID: 7}, 1, updated.ID, input, appsec.RequestMeta{}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate subscription effective date error = %v; want conflict", err)
	}
	input.PeriodAmount = "130"
	input.EffectiveAt = effectiveAt.AddDate(0, 0, 2)
	changed, err := service.UpdateSubscriptionPrice(context.Background(), admin.Identity{ID: 7}, 1, updated.ID, input, appsec.RequestMeta{})
	if err != nil || changed.PeriodAmount != "130" || !changed.EffectiveAt.Equal(input.EffectiveAt) {
		t.Fatalf("updated subscription = %+v, err = %v", changed, err)
	}
	if err := service.DeleteSubscriptionPrice(context.Background(), admin.Identity{ID: 7}, 1, changed.ID, appsec.RequestMeta{}); err != nil || len(store.subs) != 1 || store.subs[0].ID != saved.ID {
		t.Fatalf("delete subscription price: subs=%+v err=%v", store.subs, err)
	}
}

func TestReferencedSubscriptionPriceOnlyAllowsAmountCorrection(t *testing.T) {
	effectiveAt := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	store := &priceStoreStub{
		resource:   ResourceRecord{Resource: Resource{ID: 1, Name: "subscription", AuthType: "SUBSCRIPTION"}},
		referenced: true,
		subs: []SubscriptionPrice{{
			ID: 9, ProviderCredentialID: 1, Currency: "CNY", PeriodAmount: "1000",
			BillingPeriod: "MONTH", EffectiveAt: effectiveAt, CreatedAt: effectiveAt,
		}},
	}
	service := PriceService{store: store}
	input := SubscriptionPriceInput{Currency: "CNY", PeriodAmount: "800", BillingPeriod: "MONTH", EffectiveAt: effectiveAt}
	updated, err := service.UpdateSubscriptionPrice(context.Background(), admin.Identity{ID: 7}, 1, 9, input, appsec.RequestMeta{})
	if err != nil || updated.PeriodAmount != "800" {
		t.Fatalf("amount correction=%+v err=%v", updated, err)
	}
	input.Currency = "USD"
	if _, err := service.UpdateSubscriptionPrice(context.Background(), admin.Identity{ID: 7}, 1, 9, input, appsec.RequestMeta{}); !errors.Is(err, ErrConflict) {
		t.Fatalf("referenced currency update error=%v", err)
	}
	if err := service.DeleteSubscriptionPrice(context.Background(), admin.Identity{ID: 7}, 1, 9, appsec.RequestMeta{}); !errors.Is(err, ErrConflict) {
		t.Fatalf("referenced delete error=%v", err)
	}
}

func TestSubscriptionPriceChangesCannotAffectChargedPeriods(t *testing.T) {
	chargedUntil := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	existingAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	futureAt := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	newStore := func() *priceStoreStub {
		return &priceStoreStub{
			resource:     ResourceRecord{Resource: Resource{ID: 1, Name: "subscription", AuthType: "SUBSCRIPTION"}},
			chargedUntil: &chargedUntil,
			subs: []SubscriptionPrice{
				{ID: 9, ProviderCredentialID: 1, Currency: "CNY", PeriodAmount: "100", BillingPeriod: "MONTH", EffectiveAt: futureAt},
				{ID: 8, ProviderCredentialID: 1, Currency: "CNY", PeriodAmount: "100", BillingPeriod: "MONTH", EffectiveAt: existingAt},
			},
		}
	}
	ctx, actor, meta := context.Background(), admin.Identity{ID: 7}, appsec.RequestMeta{}
	for _, tt := range []struct {
		name    string
		run     func(*PriceService) error
		wantErr error
	}{
		{name: "backfill before charged period", wantErr: ErrConflict, run: func(service *PriceService) error {
			_, err := service.SaveSubscriptionPrice(ctx, actor, 1, SubscriptionPriceInput{Currency: "CNY", PeriodAmount: "90",
				BillingPeriod: "MONTH", EffectiveAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)}, meta)
			return err
		}},
		{name: "insert inside charged period", wantErr: ErrConflict, run: func(service *PriceService) error {
			_, err := service.SaveSubscriptionPrice(ctx, actor, 1, SubscriptionPriceInput{Currency: "CNY", PeriodAmount: "90",
				BillingPeriod: "MONTH", EffectiveAt: time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)}, meta)
			return err
		}},
		{name: "insert at charged boundary", run: func(service *PriceService) error {
			_, err := service.SaveSubscriptionPrice(ctx, actor, 1, SubscriptionPriceInput{Currency: "CNY", PeriodAmount: "90",
				BillingPeriod: "MONTH", EffectiveAt: chargedUntil}, meta)
			return err
		}},
		{name: "move future price into charged period", wantErr: ErrConflict, run: func(service *PriceService) error {
			_, err := service.UpdateSubscriptionPrice(ctx, actor, 1, 9, SubscriptionPriceInput{Currency: "CNY", PeriodAmount: "100",
				BillingPeriod: "MONTH", EffectiveAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}, meta)
			return err
		}},
		{name: "change charged price period", wantErr: ErrConflict, run: func(service *PriceService) error {
			_, err := service.UpdateSubscriptionPrice(ctx, actor, 1, 8, SubscriptionPriceInput{Currency: "CNY", PeriodAmount: "100",
				BillingPeriod: "YEAR", EffectiveAt: existingAt}, meta)
			return err
		}},
		{name: "correct charged price amount", run: func(service *PriceService) error {
			_, err := service.UpdateSubscriptionPrice(ctx, actor, 1, 8, SubscriptionPriceInput{Currency: "CNY", PeriodAmount: "80",
				BillingPeriod: "MONTH", EffectiveAt: existingAt}, meta)
			return err
		}},
		{name: "change future price period", run: func(service *PriceService) error {
			_, err := service.UpdateSubscriptionPrice(ctx, actor, 1, 9, SubscriptionPriceInput{Currency: "CNY", PeriodAmount: "100",
				BillingPeriod: "YEAR", EffectiveAt: futureAt}, meta)
			return err
		}},
		{name: "delete price before charged period", wantErr: ErrConflict, run: func(service *PriceService) error {
			return service.DeleteSubscriptionPrice(ctx, actor, 1, 8, meta)
		}},
		{name: "delete future price", run: func(service *PriceService) error {
			return service.DeleteSubscriptionPrice(ctx, actor, 1, 9, meta)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := newStore()
			service := &PriceService{store: store, ids: &priceIDs{next: 100}, now: func() time.Time { return chargedUntil }}
			err := tt.run(service)
			if tt.wantErr == nil && err != nil || tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("error=%v; want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil && store.audits != 0 {
				t.Fatalf("rejected change was audited: %d", store.audits)
			}
		})
	}
}

func TestModelPricePrechecksReferenceAndHistoryOverride(t *testing.T) {
	effectiveAt := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	ctx, actor, meta := context.Background(), admin.Identity{ID: 7}, appsec.RequestMeta{}
	newStore := func(referenced, overrides bool) *priceStoreStub {
		return &priceStoreStub{
			resource:        ResourceRecord{Resource: Resource{ID: 1, ProviderID: 3, Name: "key", AuthType: "API_KEY"}},
			mappings:        []ProviderMapping{{ID: 2, ProviderID: 3}},
			modelReferenced: referenced, overrides: overrides,
			models: []ModelPrice{{ID: 9, ProviderCredentialID: 1, ProviderModelID: 2, Currency: "USD",
				InputPrice: "1", OutputPrice: "2", CachedInputPrice: "0.5", EffectiveAt: effectiveAt}},
		}
	}
	input := func(currency string, at time.Time, inputPrice string) ModelPriceInput {
		return ModelPriceInput{ProviderModelID: 2, Currency: currency, InputPrice: inputPrice, OutputPrice: "2",
			CachedInputPrice: "0.5", EffectiveAt: at}
	}
	for _, tt := range []struct {
		name                  string
		referenced, overrides bool
		run                   func(*PriceService) error
		wantErr               error
	}{
		{name: "insert overriding rated history", overrides: true, wantErr: ErrConflict, run: func(service *PriceService) error {
			_, err := service.SaveModelPrice(ctx, actor, 1, input("USD", effectiveAt.AddDate(0, 0, 1), "3"), meta)
			return err
		}},
		{name: "insert future version", run: func(service *PriceService) error {
			_, err := service.SaveModelPrice(ctx, actor, 1, input("USD", effectiveAt.AddDate(0, 0, 1), "3"), meta)
			return err
		}},
		{name: "correct referenced amount", referenced: true, run: func(service *PriceService) error {
			_, err := service.UpdateModelPrice(ctx, actor, 1, 9, input("USD", effectiveAt, "3"), meta)
			return err
		}},
		{name: "change referenced currency", referenced: true, wantErr: ErrConflict, run: func(service *PriceService) error {
			_, err := service.UpdateModelPrice(ctx, actor, 1, 9, input("CNY", effectiveAt, "1"), meta)
			return err
		}},
		{name: "move referenced effective time", referenced: true, wantErr: ErrConflict, run: func(service *PriceService) error {
			_, err := service.UpdateModelPrice(ctx, actor, 1, 9, input("USD", effectiveAt.AddDate(0, 0, -1), "1"), meta)
			return err
		}},
		{name: "move unreferenced version over rated history", overrides: true, wantErr: ErrConflict, run: func(service *PriceService) error {
			_, err := service.UpdateModelPrice(ctx, actor, 1, 9, input("USD", effectiveAt.AddDate(0, 0, -1), "1"), meta)
			return err
		}},
		{name: "delete referenced version", referenced: true, wantErr: ErrConflict, run: func(service *PriceService) error {
			return service.DeleteModelPrice(ctx, actor, 1, 2, 9, meta)
		}},
		{name: "delete unreferenced version", run: func(service *PriceService) error {
			return service.DeleteModelPrice(ctx, actor, 1, 2, 9, meta)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := newStore(tt.referenced, tt.overrides)
			service := &PriceService{store: store, ids: &priceIDs{next: 100}, now: func() time.Time { return effectiveAt }}
			err := tt.run(service)
			if tt.wantErr == nil && err != nil || tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("error=%v; want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil && store.audits != 0 {
				t.Fatalf("rejected change was audited: %d", store.audits)
			}
		})
	}
}
