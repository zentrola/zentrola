package management

import (
	"context"
	"strings"
	"testing"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
)

func TestModelInputValidation(t *testing.T) {
	valid := ModelInput{Code: "official-model-v1", Name: "官方模型", InputModalities: []string{"TEXT", "IMAGE"}, OutputModalities: []string{"TEXT"}}
	if !validModelInput(valid) {
		t.Fatal("valid model rejected")
	}
	for _, values := range [][]string{nil, {}, {"TEXT", "TEXT"}, {"text"}, {"UNKNOWN"}, {"TEXT", ""}} {
		for _, input := range []bool{true, false} {
			model := valid
			if input {
				model.InputModalities = values
			} else {
				model.OutputModalities = values
			}
			if validModelInput(model) {
				t.Errorf("invalid modalities accepted: %v", values)
			}
		}
	}
	for _, mutate := range []func(*ModelInput){
		func(m *ModelInput) { m.Code = " " },
		func(m *ModelInput) { m.Name = strings.Repeat("中", 43) },
		func(m *ModelInput) { m.Remark = strings.Repeat("中", 667) },
		func(m *ModelInput) { m.Remark = "\x00" },
		func(m *ModelInput) {
			publisherID := int64(0)
			m.PublisherProviderID = &publisherID
		},
	} {
		model := valid
		mutate(&model)
		if validModelInput(model) {
			t.Errorf("invalid model accepted: %+v", model)
		}
	}
}

type modelSessionStub struct {
	ModelSession
	model    Model
	provider Provider
	created  *Model
	updated  *Model
	deleted  *time.Time
	audits   []Audit
}

func (s *modelSessionStub) Model(context.Context, int64) (Model, error) { return s.model, nil }
func (s *modelSessionStub) Provider(context.Context, int64) (Provider, error) {
	return s.provider, nil
}
func (s *modelSessionStub) CreateModel(_ context.Context, model Model) error {
	s.created = &model
	return nil
}
func (s *modelSessionStub) UpdateModel(_ context.Context, model Model) error {
	s.updated = &model
	return nil
}
func (s *modelSessionStub) DeleteModel(_ context.Context, _ int64, at time.Time) error {
	s.deleted = &at
	return nil
}
func (s *modelSessionStub) Audit(_ context.Context, audit Audit, _ appsec.RequestMeta) error {
	s.audits = append(s.audits, audit)
	return nil
}

type modelStoreStub struct{ session *modelSessionStub }

func (s modelStoreStub) ReadModel(_ context.Context, _ admin.Identity, fn func(ModelLookup) error) error {
	return fn(s.session)
}
func (s modelStoreStub) WriteModel(_ context.Context, _ admin.Identity, fn func(ModelSession) error) error {
	return fn(s.session)
}

func TestModelCRUDUsesNarrowStoreAndInjectedClock(t *testing.T) {
	publisherID := int64(3)
	fixed := time.Date(2026, 10, 2, 3, 4, 5, 987654321, time.UTC)
	session := &modelSessionStub{
		model:    Model{ID: 8, Code: "old", Name: "旧模型", Status: "DISABLED"},
		provider: Provider{ID: publisherID, Name: "发布商"},
	}
	service := ModelService{
		store: modelStoreStub{session: session}, ids: fixedMemberID{id: 8}, now: func() time.Time { return fixed },
	}
	input := ModelInput{
		Code: " model-v2 ", Name: " 新模型 ", PublisherProviderID: &publisherID,
		InputModalities: []string{" TEXT "}, OutputModalities: []string{" TEXT "},
	}
	created, err := service.CreateModel(context.Background(), admin.Identity{ID: 1}, input, appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	wantTime := fixed.Truncate(time.Microsecond)
	if session.created == nil || created.Code != "model-v2" || created.PublisherProviderName == nil ||
		*created.PublisherProviderName != "发布商" || !created.CreatedAt.Equal(wantTime) {
		t.Fatalf("unexpected created model: %+v", created)
	}
	updated, err := service.UpdateModel(context.Background(), admin.Identity{ID: 1}, 8, input, appsec.RequestMeta{})
	if err != nil || session.updated == nil || updated.Name != "新模型" || !updated.UpdatedAt.Equal(wantTime) {
		t.Fatalf("updated=%+v persisted=%+v err=%v", updated, session.updated, err)
	}
	if err := service.DeleteModel(context.Background(), admin.Identity{ID: 1}, 8, appsec.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	if session.deleted == nil || !session.deleted.Equal(wantTime) || len(session.audits) != 3 {
		t.Fatalf("deletedAt=%v audits=%d", session.deleted, len(session.audits))
	}
}

func TestModelInputNormalization(t *testing.T) {
	input := ModelInput{
		Code:             "  official-model-v1  ",
		Name:             "  官方模型  ",
		InputModalities:  []string{" TEXT ", " IMAGE "},
		OutputModalities: []string{" TEXT "},
		Remark:           "  备注  ",
	}
	input.Normalize()
	if !input.Valid() || input.Code != "official-model-v1" || input.Name != "官方模型" || input.Remark != "备注" ||
		input.InputModalities[0] != "TEXT" || input.InputModalities[1] != "IMAGE" {
		t.Fatalf("unexpected normalized model input: %+v", input)
	}
}
