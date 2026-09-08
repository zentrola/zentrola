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
		if p.ProviderType != "OFFICIAL" || p.AnthropicBaseUrl != "https://api.deepseek.com/anthropic" {
			return conflict
		}
		providerID = p.ID
		if p.OpenaiBaseUrl != "" && p.OpenaiBaseUrl != "https://api.deepseek.com" {
			return conflict
		}
	}
	at := pgtype.Timestamptz{Time: seed.CreatedAt, Valid: true}
	if !exists {
		anthropicBaseURL := "https://api.deepseek.com/anthropic"
		if err := q.CreateBootstrapProvider(ctx, dbgen.CreateBootstrapProviderParams{ID: providerID, ProviderCode: "deepseek-official", ProviderName: "DeepSeek Official", CreatedAt: at}); err != nil {
			return err
		}
		if err := q.CreateBootstrapProviderEndpoint(ctx, dbgen.CreateBootstrapProviderEndpointParams{ProviderID: providerID, ProtocolType: "ANTHROPIC_MESSAGES", BaseUrl: anthropicBaseURL, CreatedAt: at}); err != nil {
			return err
		}
	}
	if err := q.SetupDeepSeekOpenAIEndpoint(ctx, dbgen.SetupDeepSeekOpenAIEndpointParams{ProviderID: providerID, CreatedAt: at}); err != nil {
		return err
	}
	for _, m := range seed.Models {
		models, err := q.CatalogModelHistory(ctx, m.Code)
		if err != nil {
			return err
		}
		if exists {
			if len(models) != 1 {
				return conflict
			}
			mappings, err := q.CatalogMappingHistory(ctx, models[0])
			if err != nil {
				return err
			}
			if len(mappings) != 1 || mappings[0].ProviderID != providerID || mappings[0].UpstreamModelCode != m.UpstreamCode {
				return conflict
			}
			m.ID = models[0]
			if mappings[0].IsDeleted {
				return conflict
			}
		} else {
			if len(models) > 1 {
				return conflict
			}
			if len(models) == 0 {
				if err := q.CreateBootstrapModel(ctx, dbgen.CreateBootstrapModelParams{ID: m.ID, ModelCode: m.Code, DisplayName: m.Name, InputModalities: []byte(m.InputModalities), CreatedAt: at}); err != nil {
					return err
				}
			} else {
				m.ID = models[0]
			}
		}
		if !exists {
			if err := q.CreateBootstrapProviderModel(ctx, dbgen.CreateBootstrapProviderModelParams{ID: m.ProviderModelID, ProviderID: providerID, ModelID: m.ID, UpstreamModelCode: m.UpstreamCode, CreatedAt: at}); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}
