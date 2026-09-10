package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
)

func createActiveTestModel(t *testing.T, ctx context.Context, service *mgmt.Service, actor admin.Identity, code, name string, inputModalities []string) mgmt.Model {
	t.Helper()
	model, err := service.CreateModel(ctx, actor, mgmt.ModelInput{
		Code:             code,
		Name:             name,
		InputModalities:  inputModalities,
		OutputModalities: []string{"TEXT"},
	}, appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SetModelStatus(ctx, actor, model.ID, "ACTIVE", appsec.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	model.Status = "ACTIVE"
	return model
}

func createTestProvider(t *testing.T, ctx context.Context, pool *pgxpool.Pool, service *mgmt.Service, actor admin.Identity, name string, endpoints []mgmt.ProviderEndpoint, mappings []mgmt.ProviderMappingInput) mgmt.Provider {
	t.Helper()
	provider, err := service.CreateProvider(ctx, actor, mgmt.ProviderInput{
		Name:      name,
		Endpoints: endpoints,
		Mappings:  mappings,
	}, appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, query := range []string{
			"DELETE FROM provider_model WHERE provider_id=$1",
			"DELETE FROM provider_endpoint WHERE provider_id=$1",
			"DELETE FROM provider WHERE id=$1",
		} {
			if _, err := pool.Exec(cleanup, query, provider.ID); err != nil {
				t.Error(err)
				return
			}
		}
	})
	return provider
}
