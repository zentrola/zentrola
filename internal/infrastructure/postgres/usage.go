package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
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

type UsageStore struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

func NewUsageStore(pool *pgxpool.Pool, loggers ...*slog.Logger) *UsageStore {
	return &UsageStore{pool: pool, logger: optionalLogger(loggers)}
}

func (s *UsageStore) usageError(ctx context.Context, operation string, err error) error {
	return diagnosePostgresError(ctx, s.logger, "usage", operation, err, appsec.ErrUnavailable)
}
func (s *UsageStore) WriteBatch(ctx context.Context, events []domain.Event) error {
	attempts := make([]map[string]any, 0, len(events))
	optionalError := func(e string) any {
		if e == "" {
			return nil
		}
		return e
	}
	for _, e := range events {
		if e.ClientProtocol == "" {
			e.ClientProtocol = "ANTHROPIC_MESSAGES"
		}
		all := append([]domain.Attempt(nil), e.Attempts...)
		if e.Attempt != nil {
			all = append(all, *e.Attempt)
		}
		for index := range all {
			a := all[index]
			attemptNo := a.AttemptNo
			if attemptNo <= 0 {
				attemptNo = int32(index + 1)
			}
			attempts = append(attempts, map[string]any{"request_id": e.RequestID, "attempt_no": attemptNo, "client_protocol": e.ClientProtocol, "principal_id": e.PrincipalID, "provider_id": a.ProviderID, "provider_model_id": a.ProviderModelID, "provider_credential_id": a.ResourceID, "model_id": a.ModelID, "input_tokens": a.InputTokens, "output_tokens": a.OutputTokens, "cached_input_tokens": a.CachedInputTokens, "started_at": a.StartedAt, "completed_at": a.CompletedAt, "latency_ms": a.CompletedAt.Sub(a.StartedAt).Milliseconds(), "status": a.Status, "error_type": optionalError(a.ErrorType)})
		}
	}
	if len(attempts) == 0 {
		return nil
	}
	rawAttempts, err := json.Marshal(attempts)
	if err != nil {
		return errors.New("invalid usage events")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return diagnosePostgresError(ctx, s.logger, "usage", "begin_write_batch", err, errors.New("usage database unavailable"))
	}
	defer tx.Rollback(context.Background())
	q := dbgen.New(tx)
	if err = q.InsertUsageAttempts(ctx, rawAttempts); err != nil {
		return diagnosePostgresError(ctx, s.logger, "usage", "insert_usage_attempts", err, errors.New("usage attempt insert failed"))
	}
	if err = tx.Commit(ctx); err != nil {
		return diagnosePostgresError(ctx, s.logger, "usage", "commit_write_batch", err, errors.New("usage transaction commit failed"))
	}
	return nil
}
func (s *UsageStore) Query(ctx context.Context, actor admin.Identity, f app.Filter) (app.Page, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return app.Page{}, s.usageError(ctx, "begin_usage_query", err)
	}
	defer tx.Rollback(context.Background())
	q := dbgen.New(tx)
	if err = validateActor(ctx, q, actor); err != nil {
		mapped := managementError(err)
		if errors.Is(mapped, appsec.ErrUnavailable) {
			return app.Page{}, s.usageError(ctx, "validate_usage_actor", err)
		}
		return app.Page{}, mapped
	}
	ts := func(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
	pageLimit := f.Limit
	if f.ProbeNext {
		pageLimit++
	}
	rows, err := q.QueryUsage(ctx, dbgen.QueryUsageParams{AfterID: f.After, FromTime: ts(f.From), ToTime: ts(f.To), PrincipalID: f.PrincipalID, ModelID: f.ModelID, ProviderID: f.ProviderID, ResourceID: f.ResourceID, PageLimit: pageLimit})
	if err != nil {
		return app.Page{}, s.usageError(ctx, "query_usage", err)
	}
	result := make([]app.Row, 0, len(rows))
	for _, r := range rows {
		result = append(result, app.Row{ID: r.ID, RequestID: r.RequestID, ClientProtocol: r.ClientProtocol, PrincipalID: r.PrincipalID, PrincipalName: r.PrincipalName, ModelID: r.ModelID, RequestAt: r.StartedAt.Time.UTC(), CompletedAt: r.CompletedAt.Time.UTC(), LatencyMS: r.LatencyMs, Status: r.Status, ErrorType: r.ErrorType, AttemptNo: r.AttemptNo, ProviderID: r.ProviderID, ProviderModelID: r.ProviderModelID, ResourceID: r.ResourceID, InputTokens: r.InputTokens, OutputTokens: r.OutputTokens, CachedInputTokens: r.CachedInputTokens})
	}
	total, err := q.CountUsage(ctx, dbgen.CountUsageParams{FromTime: ts(f.From), ToTime: ts(f.To), PrincipalID: f.PrincipalID, ModelID: f.ModelID, ProviderID: f.ProviderID, ResourceID: f.ResourceID})
	if err != nil {
		return app.Page{}, s.usageError(ctx, "count_usage", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return app.Page{}, s.usageError(ctx, "commit_usage_query", err)
	}
	return app.Page{Items: result, Total: total}, nil
}

func (s *UsageStore) TokenUsage(ctx context.Context, principalID int64, from, to time.Time) (int64, error) {
	ts := func(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
	tokens, err := dbgen.New(s.pool).GetPrincipalTokenUsage(ctx, dbgen.GetPrincipalTokenUsageParams{
		PrincipalID: principalID,
		FromTime:    ts(from),
		ToTime:      ts(to),
	})
	if err != nil {
		return 0, s.usageError(ctx, "get_principal_token_usage", err)
	}
	return tokens, nil
}

func (s *UsageStore) Statistics(ctx context.Context, actor admin.Identity, f app.StatisticFilter) (app.StatisticPage, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return app.StatisticPage{}, s.usageError(ctx, "begin_usage_statistics", err)
	}
	defer tx.Rollback(context.Background())
	q := dbgen.New(tx)
	if err = validateActor(ctx, q, actor); err != nil {
		mapped := managementError(err)
		if errors.Is(mapped, appsec.ErrUnavailable) {
			return app.StatisticPage{}, s.usageError(ctx, "validate_statistics_actor", err)
		}
		return app.StatisticPage{}, mapped
	}
	ts := func(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
	pageLimit := f.Limit
	if f.ProbeNext {
		pageLimit++
	}
	result := make([]app.StatisticRow, 0, pageLimit)
	var total int64
	appendRow := func(entityID int64, name, code string, metricCount, successful, inputTokens, outputTokens, cachedInputTokens, tokens, overallTokens, averageLatencyMS, totalCount int64) {
		result = append(result, app.StatisticRow{
			EntityID: entityID, Name: name, Code: code, Count: metricCount,
			Successful: successful, InputTokens: inputTokens, OutputTokens: outputTokens,
			CachedInputTokens: cachedInputTokens, Tokens: tokens, OverallTokens: overallTokens,
			AverageLatencyMS: averageLatencyMS,
		})
		total = totalCount
	}
	switch f.Dimension {
	case app.StatisticMember:
		rows, queryErr := q.UsageMemberStatistics(ctx, dbgen.UsageMemberStatisticsParams{FromTime: ts(f.From), ToTime: ts(f.To), PageOffset: f.After, PageLimit: pageLimit})
		if queryErr != nil {
			return app.StatisticPage{}, s.usageError(ctx, "query_member_statistics", queryErr)
		}
		for _, row := range rows {
			appendRow(row.EntityID, row.Name, row.Code, row.MetricCount, row.Successful, row.InputTokens, row.OutputTokens, row.CachedInputTokens, row.Tokens, row.OverallTokens, row.AverageLatencyMs, row.TotalCount)
		}
	case app.StatisticModel:
		rows, queryErr := q.UsageModelStatistics(ctx, dbgen.UsageModelStatisticsParams{FromTime: ts(f.From), ToTime: ts(f.To), PageOffset: f.After, PageLimit: pageLimit})
		if queryErr != nil {
			return app.StatisticPage{}, s.usageError(ctx, "query_model_statistics", queryErr)
		}
		for _, row := range rows {
			appendRow(row.EntityID, row.Name, row.Code, row.MetricCount, row.Successful, row.InputTokens, row.OutputTokens, row.CachedInputTokens, row.Tokens, row.OverallTokens, row.AverageLatencyMs, row.TotalCount)
		}
	case app.StatisticProvider:
		rows, queryErr := q.UsageProviderStatistics(ctx, dbgen.UsageProviderStatisticsParams{FromTime: ts(f.From), ToTime: ts(f.To), PageOffset: f.After, PageLimit: pageLimit})
		if queryErr != nil {
			return app.StatisticPage{}, s.usageError(ctx, "query_provider_statistics", queryErr)
		}
		for _, row := range rows {
			appendRow(row.EntityID, row.Name, row.Code, row.MetricCount, row.Successful, row.InputTokens, row.OutputTokens, row.CachedInputTokens, row.Tokens, row.OverallTokens, row.AverageLatencyMs, row.TotalCount)
		}
	default:
		return app.StatisticPage{}, appsec.ErrInvalidArgument
	}
	if err = tx.Commit(ctx); err != nil {
		return app.StatisticPage{}, s.usageError(ctx, "commit_usage_statistics", err)
	}
	return app.StatisticPage{Items: result, Total: total}, nil
}

func (s *UsageStore) Dashboard(ctx context.Context, actor admin.Identity, from, to time.Time) (app.Dashboard, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return app.Dashboard{}, s.usageError(ctx, "begin_usage_dashboard", err)
	}
	defer tx.Rollback(context.Background())
	q := dbgen.New(tx)
	if err = validateActor(ctx, q, actor); err != nil {
		mapped := managementError(err)
		if errors.Is(mapped, appsec.ErrUnavailable) {
			return app.Dashboard{}, s.usageError(ctx, "validate_dashboard_actor", err)
		}
		return app.Dashboard{}, mapped
	}
	ts := func(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
	params := dbgen.UsageDashboardCountsParams{
		FromTime: ts(from),
		ToTime:   ts(to),
	}
	counts, err := q.UsageDashboardCounts(ctx, params)
	if err != nil {
		return app.Dashboard{}, s.usageError(ctx, "query_dashboard_counts", err)
	}
	tokenRows, err := q.UsageTokenRanking(ctx, dbgen.UsageTokenRankingParams(params))
	if err != nil {
		return app.Dashboard{}, s.usageError(ctx, "query_token_ranking", err)
	}
	clientModelRows, err := q.UsageClientModelRanking(ctx, dbgen.UsageClientModelRankingParams(params))
	if err != nil {
		return app.Dashboard{}, s.usageError(ctx, "query_client_model_ranking", err)
	}
	providerRows, err := q.UsageProviderRanking(ctx, dbgen.UsageProviderRankingParams(params))
	if err != nil {
		return app.Dashboard{}, s.usageError(ctx, "query_provider_ranking", err)
	}
	result := app.Dashboard{
		ActiveMemberCount:  counts.ActiveMemberCount,
		ModelCount:         counts.ModelCount,
		ProviderCount:      counts.ProviderCount,
		TotalTokens:        counts.TotalTokens,
		TokenRanking:       make([]app.TokenRank, 0, len(tokenRows)),
		ClientModelRanking: make([]app.ClientModelRank, 0, len(clientModelRows)),
		ProviderRanking:    make([]app.ProviderRank, 0, len(providerRows)),
	}
	for _, row := range tokenRows {
		result.TokenRanking = append(result.TokenRanking, app.TokenRank{
			PrincipalID: row.PrincipalID,
			Name:        row.Name,
			Tokens:      row.Tokens,
		})
	}
	for _, row := range clientModelRows {
		result.ClientModelRanking = append(result.ClientModelRanking, app.ClientModelRank{
			ModelID:   row.ModelID,
			ModelCode: row.ModelCode,
			ModelName: row.ModelName,
			Requests:  row.Requests,
			Tokens:    row.Tokens,
		})
	}
	for _, row := range providerRows {
		result.ProviderRanking = append(result.ProviderRanking, app.ProviderRank{
			ProviderID:   row.ProviderID,
			ProviderName: row.ProviderName,
			Calls:        row.Calls,
			Tokens:       row.Tokens,
		})
	}
	if err = tx.Commit(ctx); err != nil {
		return app.Dashboard{}, s.usageError(ctx, "commit_usage_dashboard", err)
	}
	return result, nil
}
