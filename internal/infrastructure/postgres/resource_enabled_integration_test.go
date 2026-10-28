package postgres

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
)

func TestCredentialEnabledSwitchesGatewayCandidateAndKeepsSubscriptionRefresh(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	for _, statement := range []string{
		`INSERT INTO provider (id,provider_code,provider_name,provider_type,status,created_by,updated_by,created_at,updated_at)
		 VALUES (40,'switch-test','Switch Test','OFFICIAL','ACTIVE','system','system',now(),now())`,
		`INSERT INTO model (id,status,model_code,display_name,input_modalities,output_modalities,created_by,updated_by,created_at,updated_at)
		 VALUES (41,'ACTIVE','switch-model','Switch Model','["TEXT"]','["TEXT"]','system','system',now(),now())`,
		`INSERT INTO provider_endpoint (provider_id,protocol_type,base_url,created_by,updated_by,created_at,updated_at)
		 VALUES (40,'OPENAI','https://example.com','system','system',now(),now())`,
		`INSERT INTO provider_model (id,provider_id,model_id,upstream_model_code,priority,created_by,updated_by,created_at,updated_at)
		 VALUES (42,40,41,'',100,'system','system',now(),now())`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO provider_credential
		(id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,auth_type,auth_adapter,subscription_type,quota_status,created_by,updated_by,created_at,updated_at)
		VALUES (60,40,'subscription',$1,$2,1,'SUBSCRIPTION','OPENAI_CODEX','PERSONAL','AVAILABLE','system','system',now(),now()),
		       (61,40,'api-key',$1,$2,1,'API_KEY','API_KEY',NULL,'UNKNOWN','system','system',now(),now())`,
		bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 12)); err != nil {
		t.Fatal(err)
	}
	queries := dbgen.New(pool)
	selected := func(want int64) {
		t.Helper()
		candidates, err := queries.GatewayCandidates(ctx, dbgen.GatewayCandidatesParams{ModelID: 41, PreferredProtocol: "OPENAI"})
		if err != nil {
			t.Fatal(err)
		}
		if len(candidates) == 0 || candidates[0].ResourceID != want {
			t.Fatalf("first candidate=%+v, want resource %d", candidates, want)
		}
	}
	selected(60)
	session := &managementSession{q: queries, actor: admin.Identity{ID: 1}}
	now := time.Now().UTC()
	if err := session.SetResourceEnabled(ctx, 60, 1, false, now); err != nil {
		t.Fatal(err)
	}
	selected(61)
	if err := session.SetResourceEnabled(ctx, 61, 1, false, now); err != nil {
		t.Fatal(err)
	}
	noCandidates, err := queries.GatewayCandidates(ctx, dbgen.GatewayCandidatesParams{ModelID: 41, PreferredProtocol: "OPENAI"})
	if err != nil || len(noCandidates) != 0 {
		t.Fatalf("all credentials disabled, candidates=%+v error=%v", noCandidates, err)
	}
	configured, err := queries.ManageProviderCredentialConfigured(ctx, 40)
	if err != nil || configured {
		t.Fatalf("all credentials disabled, provider configured=%v error=%v", configured, err)
	}
	if err := session.SetResourceEnabled(ctx, 61, 2, true, now); err != nil {
		t.Fatal(err)
	}
	resource, err := session.Resource(ctx, 60)
	if err != nil || resource.IsEnabled || resource.Version != 2 {
		t.Fatalf("disabled resource=%+v error=%v", resource, err)
	}
	refreshable, err := queries.ListGatewaySubscriptionCredentials(ctx, dbgen.ListGatewaySubscriptionCredentialsParams{
		RefreshBefore: pgtype.Timestamptz{Time: now, Valid: true}, PageLimit: 10,
	})
	if err != nil || len(refreshable) != 1 || refreshable[0].ResourceID != 60 {
		t.Fatalf("disabled subscription refresh list=%+v error=%v", refreshable, err)
	}
	if err := session.SetResourceEnabled(ctx, 60, 1, true, now); !errors.Is(err, mgmt.ErrConflict) {
		t.Fatalf("stale status update error=%v", err)
	}
	if err := session.SetResourceEnabled(ctx, 60, 2, true, now); err != nil {
		t.Fatal(err)
	}
	selected(60)
	if err := session.BlockResourceRuntime(ctx, 60, 3, "BILLING", "UPSTREAM_BILLING_BLOCKED", nil, now); err != nil {
		t.Fatal(err)
	}
	selected(61)
	resource, err = session.Resource(ctx, 60)
	if err != nil || !resource.IsEnabled || resource.RuntimeStatus != "BLOCKED" {
		t.Fatalf("runtime block and manual state=%+v error=%v", resource, err)
	}
}
