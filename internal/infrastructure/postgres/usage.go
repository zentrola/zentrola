package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	app "github.com/zentrola/zentrola/internal/application/usage"
	"github.com/zentrola/zentrola/internal/domain/admin"
	domain "github.com/zentrola/zentrola/internal/domain/usage"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
)

type UsageStore struct{ pool *pgxpool.Pool }

func NewUsageStore(pool *pgxpool.Pool) *UsageStore { return &UsageStore{pool} }
func (s *UsageStore) WriteBatch(ctx context.Context, events []domain.Event) error {
	if len(events) == 0 {
		return nil
	}
	requests := make([]map[string]any, 0, len(events))
	attempts := make([]map[string]any, 0, len(events))
	optionalID := func(n int64) any {
		if n == 0 {
			return nil
		}
		return n
	}
	optionalError := func(e string) any {
		if e == "" {
			return nil
		}
		return e
	}
	for _, e := range events {
		if e.ClientProtocol == "" {
			e.ClientProtocol = "ANTHROPIC"
		}
		requests = append(requests, map[string]any{"id": e.ID, "organization_id": e.OrganizationID, "request_id": e.RequestID, "client_protocol": e.ClientProtocol, "principal_id": e.PrincipalID, "model_id": optionalID(e.ModelID), "request_at": e.RequestAt, "completed_at": e.CompletedAt, "latency_ms": e.CompletedAt.Sub(e.RequestAt).Milliseconds(), "status": e.Status, "error_type": optionalError(e.ErrorType)})
		if a := e.Attempt; a != nil {
			attempts = append(attempts, map[string]any{"id": a.ID, "organization_id": e.OrganizationID, "request_id": e.RequestID, "client_protocol": e.ClientProtocol, "principal_id": e.PrincipalID, "provider_id": a.ProviderID, "provider_model_id": a.ProviderModelID, "resource_id": a.ResourceID, "model_id": a.ModelID, "input_tokens": a.InputTokens, "output_tokens": a.OutputTokens, "cached_input_tokens": a.CachedInputTokens, "started_at": a.StartedAt, "completed_at": a.CompletedAt, "latency_ms": a.CompletedAt.Sub(a.StartedAt).Milliseconds(), "status": a.Status, "error_type": optionalError(a.ErrorType)})
		}
	}
	rawRequests, err := json.Marshal(requests)
	if err != nil {
		return errors.New("invalid usage events")
	}
	rawAttempts, err := json.Marshal(attempts)
	if err != nil {
		return errors.New("invalid usage events")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return errors.New("usage database unavailable")
	}
	defer tx.Rollback(context.Background())
	q := dbgen.New(tx)
	if err = q.InsertUsageRequests(ctx, rawRequests); err != nil {
		return errors.New("usage request insert failed")
	}
	if len(attempts) > 0 {
		if err = q.InsertUsageAttempts(ctx, rawAttempts); err != nil {
			return errors.New("usage attempt insert failed")
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return errors.New("usage transaction commit failed")
	}
	return nil
}
func (s *UsageStore) Query(ctx context.Context, actor admin.Identity, f app.Filter) ([]app.Row, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, appsec.ErrUnavailable
	}
	defer tx.Rollback(context.Background())
	q := dbgen.New(tx)
	if err = validateActor(ctx, q, actor); err != nil {
		return nil, managementError(err)
	}
	ts := func(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
	rows, err := q.QueryUsage(ctx, dbgen.QueryUsageParams{OrganizationID: actor.OrganizationID, AfterID: f.After, FromTime: ts(f.From), ToTime: ts(f.To), PrincipalID: f.PrincipalID, ModelID: f.ModelID, ResourceID: f.ResourceID, PageLimit: f.Limit})
	if err != nil {
		return nil, appsec.ErrUnavailable
	}
	result := make([]app.Row, 0, len(rows))
	for _, r := range rows {
		result = append(result, app.Row{ID: r.ID, RequestID: r.RequestID, ClientProtocol: r.ClientProtocol, PrincipalID: r.PrincipalID, ModelID: r.ModelID, RequestAt: r.RequestAt.Time, CompletedAt: r.CompletedAt.Time, LatencyMS: r.LatencyMs, Status: r.Status, ErrorType: r.ErrorType, UsageID: r.UsageID, AttemptNo: r.AttemptNo, ProviderID: r.ProviderID, ProviderModelID: r.ProviderModelID, ResourceID: r.ResourceID, InputTokens: r.InputTokens, OutputTokens: r.OutputTokens, CachedInputTokens: r.CachedInputTokens, AttemptStatus: r.AttemptStatus, AttemptErrorType: r.AttemptErrorType})
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, appsec.ErrUnavailable
	}
	return result, nil
}
