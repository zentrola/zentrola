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
