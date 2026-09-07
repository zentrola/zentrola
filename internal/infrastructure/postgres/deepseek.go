package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zentrola/zentrola/internal/application/bootstrap"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
)

func (s *BootstrapStore) InstallDeepSeek(ctx context.Context, seed bootstrap.Seed) error {
	if err := s.installDeepSeek(ctx, seed); err != nil {
		return errors.New("DeepSeek catalog setup failed; check initialization and conflicting catalog history")
	}
	return nil
}

func (s *BootstrapStore) installDeepSeek(ctx context.Context, seed bootstrap.Seed) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(829314002)"); err != nil {
		return err
	}
	q := dbgen.New(tx)
	initialized, err := q.BootstrapInitialized(ctx)
	if err != nil {
		return err
	}
	if !initialized {
		return errors.New("initialize the organization first")
	}
	providers, err := q.DeepSeekProviderHistory(ctx)
	if err != nil {
		return err
	}
	conflict := errors.New("conflicting DeepSeek catalog history")
	if len(providers) > 1 {
		return conflict
	}
	exists := len(providers) == 1
	providerID := seed.ProviderID
	if exists {
		p := providers[0]
		if p.ProviderType != "OFFICIAL" || p.AnthropicBaseUrl == nil || *p.AnthropicBaseUrl != "https://api.deepseek.com/anthropic" {
			return conflict
		}
		providerID = p.ID
		if p.OpenaiBaseUrl != nil && *p.OpenaiBaseUrl != "https://api.deepseek.com" {
			return conflict
		}
	}
	at := pgtype.Timestamptz{Time: seed.CreatedAt, Valid: true}
	if !exists {
		anthropicBaseURL := "https://api.deepseek.com/anthropic"
		if err := q.CreateBootstrapProvider(ctx, dbgen.CreateBootstrapProviderParams{ID: providerID, ProviderCode: "deepseek-official", ProviderName: "DeepSeek Official", AnthropicBaseUrl: &anthropicBaseURL, CreatedAt: at}); err != nil {
			return err
		}
	}
	if err := q.SetupDeepSeekOpenAIURL(ctx, providerID); err != nil {
		return err
	}
	for _, m := range seed.Models {
		models, err := q.CatalogModelHistory(ctx, m.Code)
		if err != nil {
			return err
		}
		status := "ACTIVE"
		if exists {
			if len(models) != 1 {
				return conflict
			}
			mappings, err := q.CatalogMappingHistory(ctx, dbgen.CatalogMappingHistoryParams{ModelID: models[0], ProtocolType: "ANTHROPIC"})
			if err != nil {
				return err
			}
			if len(mappings) != 1 || mappings[0].ProviderID != providerID || mappings[0].UpstreamModelCode != m.UpstreamCode || mappings[0].ProtocolType != "ANTHROPIC" {
				return conflict
			}
			m.ID = models[0]
			status = mappings[0].Status
			if mappings[0].IsDeleted {
				status = "DISABLED"
			}
		} else {
			if len(models) != 0 {
				return conflict
			}
			if err := q.CreateBootstrapModel(ctx, dbgen.CreateBootstrapModelParams{ID: m.ID, ModelCode: m.Code, DisplayName: m.Name, InputModalities: []byte(`["TEXT"]`), CreatedAt: at}); err != nil {
				return err
			}
			if err := q.CreateBootstrapProviderModel(ctx, dbgen.CreateBootstrapProviderModelParams{ID: m.ProviderModelID, ProviderID: providerID, ModelID: m.ID, UpstreamModelCode: m.UpstreamCode, CreatedAt: at}); err != nil {
				return err
			}
		}
		openai, err := q.CatalogMappingHistory(ctx, dbgen.CatalogMappingHistoryParams{ModelID: m.ID, ProtocolType: "OPENAI"})
		if err != nil {
			return err
		}
		if len(openai) > 1 {
			return conflict
		}
		if len(openai) == 1 {
			if openai[0].ProviderID != providerID || openai[0].UpstreamModelCode != m.UpstreamCode {
				return conflict
			}
			continue
		}
		if err := q.CreateDeepSeekOpenAIModel(ctx, dbgen.CreateDeepSeekOpenAIModelParams{ID: m.OpenAIProviderModelID, ProviderID: providerID, ModelID: m.ID, UpstreamModelCode: m.UpstreamCode, Status: status, CreatedAt: at}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
