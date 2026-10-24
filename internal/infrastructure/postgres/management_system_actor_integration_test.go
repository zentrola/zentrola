package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/operation"
	"github.com/zentrola/zentrola/internal/infrastructure/idgen"
)

func TestManagementStoreAcceptsOnlyExplicitSystemActorIntegration(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	now := time.Date(2026, 10, 24, 2, 0, 0, 0, time.UTC)
	for _, statement := range []string{
		`INSERT INTO provider (id,provider_code,provider_name,provider_type,status,created_by,updated_by,created_at,updated_at)
         VALUES (3501,'system-provider','System Provider','OFFICIAL','ACTIVE','system','system',$1,$1)`,
		`INSERT INTO provider_credential (
             id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,
             auth_type,auth_adapter,subscription_type,created_by,updated_by,created_at,updated_at
         ) VALUES (5501,3501,'System Subscription',decode(repeat('51',32),'hex'),decode(repeat('51',12),'hex'),1,
             'SUBSCRIPTION','OPENAI_CODEX','PERSONAL','admin:1','admin:1',$1,$1)`,
	} {
		if _, err := pool.Exec(ctx, statement, now); err != nil {
			t.Fatal(err)
		}
	}
	store := NewManagementStore(pool, idgen.New(pool))
	read := func(actor admin.Identity) error {
		return store.Read(ctx, actor, func(r mgmt.Reader) error {
			_, err := r.(mgmt.ResourceReadSession).Resource(ctx, 5501)
			return err
		})
	}
	for name, actor := range map[string]admin.Identity{
		"zero identity without system flag": {DisplayName: "Member Key #7"},
		"system flag with an admin ID":      {System: true, ID: 1, DisplayName: "Member Key #7"},
	} {
		if err := read(actor); !errors.Is(err, appsec.ErrUnauthenticated) {
			t.Fatalf("%s must be rejected, got %v", name, err)
		}
	}

	actor := admin.Identity{System: true, DisplayName: "Member Key #7"}
	err := store.Write(ctx, actor, func(w mgmt.Writer) error {
		session := w.(mgmt.ResourceSession)
		current, err := session.Resource(ctx, 5501)
		if err != nil {
			return err
		}
		current.QuotaStatus, current.QuotaCheckedAt, current.UpdatedAt = mgmt.QuotaAvailable, &now, now
		if err := session.UpdateResource(ctx, current); err != nil {
			return err
		}
		return session.Audit(context.WithoutCancel(ctx), mgmt.Audit{
			Event: operation.ResourceQuotaRefresh, Target: "RESOURCE", ID: 5501, Name: current.Name,
		}, appsec.RequestMeta{})
	})
	if err != nil {
		t.Fatal(err)
	}
	var updatedBy, operatorType, operatorName string
	var checkedAt *time.Time
	if err := pool.QueryRow(ctx, `SELECT updated_by,quota_checked_at FROM provider_credential WHERE id=5501`).Scan(&updatedBy, &checkedAt); err != nil {
		t.Fatal(err)
	}
	if updatedBy != "system" || checkedAt == nil || !checkedAt.Equal(now) {
		t.Fatalf("updated_by=%q quota_checked_at=%v", updatedBy, checkedAt)
	}
	if err := pool.QueryRow(ctx, `SELECT operator_type,operator_name FROM operation_log
        WHERE operation_type='RESOURCE_QUOTA_REFRESH' AND target_id=5501`).Scan(&operatorType, &operatorName); err != nil {
		t.Fatal(err)
	}
	if operatorType != "SYSTEM" || operatorName != "Member Key #7" {
		t.Fatalf("operator=%s %q", operatorType, operatorName)
	}
}
