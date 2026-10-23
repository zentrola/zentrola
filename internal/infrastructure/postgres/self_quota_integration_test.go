package postgres

import (
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestSubscriptionQuotasListsAuthorizedProviderAccountsIntegration(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	withFixture(t, ctx, pool, func(tx pgx.Tx) {
		mustExec(t, ctx, tx, `INSERT INTO provider (id,provider_code,provider_name,provider_type,status,created_by,updated_by,created_at,updated_at)
VALUES (41,'openai','OpenAI','OFFICIAL','ACTIVE','system','system',now(),now()),(42,'other','Other','OFFICIAL','ACTIVE','system','system',now(),now());
INSERT INTO model (id,model_code,display_name,input_modalities,output_modalities,status,created_by,updated_by,created_at,updated_at)
VALUES (32,'gpt','GPT','["TEXT"]','["TEXT"]','ACTIVE','system','system',now(),now());
INSERT INTO provider_model (id,provider_id,model_id,upstream_model_code,priority,created_by,updated_by,created_at,updated_at)
VALUES (52,41,32,'gpt-upstream',100,'system','system',now(),now()),(53,42,31,'opus-upstream',100,'system','system',now(),now());
INSERT INTO principal_group_membership (id,principal_id,group_id,created_by,updated_by,created_at,updated_at)
VALUES (80,10,20,'system','system',now(),now());
INSERT INTO principal_group_model_permission (id,group_id,model_id,created_by,updated_by,created_at,updated_at)
VALUES (90,20,30,'system','system',now(),now()),(91,20,32,'system','system',now(),now());
INSERT INTO provider_credential (id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,auth_type,auth_adapter,subscription_type,quota_checked_at,created_by,updated_by,created_at,updated_at)
VALUES (60,40,'Claude A',decode(repeat('00',32),'hex'),decode(repeat('00',12),'hex'),1,'SUBSCRIPTION','TEST','PERSONAL','2026-10-24T01:00:00Z','system','system',now(),now()),
       (62,41,'Codex A',decode(repeat('00',32),'hex'),decode(repeat('00',12),'hex'),1,'SUBSCRIPTION','TEST','PERSONAL',NULL,'system','system',now(),now()),
       (63,42,'Unauthorized',decode(repeat('00',32),'hex'),decode(repeat('00',12),'hex'),1,'SUBSCRIPTION','TEST','PERSONAL',NULL,'system','system',now(),now());
INSERT INTO provider_credential (id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,created_by,updated_by,created_at,updated_at)
VALUES (61,40,'API Key',decode(repeat('00',32),'hex'),decode(repeat('00',12),'hex'),1,'system','system',now(),now());
INSERT INTO provider_credential_quota (provider_credential_id,quota_code,quota_status,used_percent,window_duration_seconds,resets_at,observed_at,created_at,updated_at)
VALUES (60,'seven_day','AVAILABLE',1,604800,'2026-10-30T02:45:00Z','2026-10-24T01:00:00Z',now(),now())`)

		result, err := subscriptionQuotas(ctx, tx, slog.Default(), 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(result) != 2 || result[0].SubscriptionName != "Claude A" || result[1].SubscriptionName != "Codex A" || result[1].ProviderName != "OpenAI" {
			t.Fatalf("unexpected accounts: %+v", result)
		}
		claude := result[0]
		if claude.Status != "KNOWN" || claude.CheckedAt == nil || claude.CheckedAt.Location().String() != "UTC" || len(claude.Quotas) != 1 {
			t.Fatalf("unexpected claude quota: %+v", claude)
		}
		item := claude.Quotas[0]
		if item.UsedPercent == nil || *item.UsedPercent != 1 || item.WindowDurationSeconds == nil || *item.WindowDurationSeconds != 604800 || item.ResetsAt == nil {
			t.Fatalf("unexpected quota item: %+v", item)
		}
		if result[1].Status != "UNKNOWN" || result[1].CheckedAt != nil || result[1].Quotas == nil || len(result[1].Quotas) != 0 {
			t.Fatalf("unchecked account must be unknown: %+v", result[1])
		}

		mustExec(t, ctx, tx, `UPDATE principal_group_membership SET is_deleted=true WHERE id=80`)
		if result, err := subscriptionQuotas(ctx, tx, slog.Default(), 10); err != nil || len(result) != 0 {
			t.Fatalf("revoked membership must hide accounts: result=%+v err=%v", result, err)
		}
	})
}
