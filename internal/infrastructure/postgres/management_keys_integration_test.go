package postgres

import (
	"bytes"
	"context"
	"testing"
	"time"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
)

func TestManageKeysExpiryFilterIntegration(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	at := time.Date(2030, time.January, 1, 0, 0, 0, 0, time.UTC)
	createdAt := at.Add(-2 * time.Hour)
	session := &managementSession{q: dbgen.New(pool), readAt: at}

	for _, test := range []struct {
		name          string
		principalType string
		principalID   int64
		keyBase       int64
		list          func(context.Context, int64, mgmt.Page, bool) ([]mgmt.Key, error)
		count         func(context.Context, int64, bool) (int64, error)
	}{
		{"member", "MEMBER", 10, 100, session.Keys, session.CountKeys},
		{"application", "APPLICATION", 20, 200, session.ApplicationKeys, session.CountApplicationKeys},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, `INSERT INTO principal
				(id,principal_type,name,status,created_by,updated_by,created_at,updated_at)
				VALUES ($1,$2,$3,'ACTIVE','system','system',$4,$4)`, test.principalID, test.principalType, test.name, createdAt); err != nil {
				t.Fatal(err)
			}

			for _, key := range []struct {
				id        int64
				status    string
				expiresAt any
				revokedAt any
			}{
				{test.keyBase + 1, "ACTIVE", at.Add(-time.Second), nil},
				{test.keyBase + 2, "REVOKED", nil, at.Add(-time.Minute)},
				{test.keyBase + 3, "ACTIVE", at, nil},
				{test.keyBase + 4, "ACTIVE", at.Add(time.Hour), nil},
				{test.keyBase + 5, "ACTIVE", nil, nil},
			} {
				if _, err := pool.Exec(ctx, `INSERT INTO principal_access_key
					(id,principal_id,key_hash,masked_key,name,status,expires_at,revoked_at,created_by,updated_by,created_at,updated_at)
					VALUES ($1,$2,$3,'test****','Test Key',$4,$5,$6,'system','system',$7,$7)`,
					key.id, test.principalID, bytes.Repeat([]byte{byte(key.id)}, 32), key.status, key.expiresAt, key.revokedAt, createdAt); err != nil {
					t.Fatal(err)
				}
			}

			total, err := test.count(ctx, test.principalID, false)
			if err != nil || total != 2 {
				t.Fatalf("unexpired key total = %d, error = %v; want 2", total, err)
			}
			for _, page := range []struct {
				after int64
				want  int64
			}{
				{0, test.keyBase + 5},
				{test.keyBase + 5, test.keyBase + 4},
			} {
				items, err := test.list(ctx, test.principalID, mgmt.Page{After: page.after, Limit: 1}, false)
				if err != nil || len(items) != 1 || items[0].ID != page.want {
					t.Fatalf("after %d: items = %+v, error = %v; want key %d", page.after, items, err, page.want)
				}
			}
			allTotal, err := test.count(ctx, test.principalID, true)
			if err != nil || allTotal != 5 {
				t.Fatalf("all key total = %d, error = %v; want 5", allTotal, err)
			}
			allItems, err := test.list(ctx, test.principalID, mgmt.Page{Limit: 5}, true)
			if err != nil || len(allItems) != 5 || allItems[2].ID != test.keyBase+3 || allItems[3].ID != test.keyBase+2 {
				t.Fatalf("all keys must include expired records in ID order: %+v, error = %v", allItems, err)
			}
			if err := session.q.ManageRevokeMemberKeys(ctx, dbgen.ManageRevokeMemberKeysParams{
				PrincipalID: test.principalID, UpdatedBy: "system", RevokedAt: pgTime(at),
			}); err != nil {
				t.Fatal(err)
			}
			var synchronized int
			if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM principal_access_key
				WHERE principal_id=$1 AND revoked_at=$2 AND expires_at=$2 AND status='REVOKED'`, test.principalID, at).Scan(&synchronized); err != nil || synchronized != 4 {
				t.Fatalf("batch revoke must set expiry and revocation to same time: count=%d err=%v", synchronized, err)
			}
		})
	}
}

func TestRevokedKeyExpiryMigrationIntegration(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	if _, err := pool.Exec(ctx, `DELETE FROM goose_db_version WHERE version_id=53 AND is_applied`); err != nil {
		t.Fatal(err)
	}
	revokedAt := time.Date(2026, time.October, 9, 8, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `INSERT INTO principal
		(id,principal_type,name,status,created_by,updated_by,created_at,updated_at)
		VALUES (30,'MEMBER','migration test','ACTIVE','system','system',$1,$1)`, revokedAt); err != nil {
		t.Fatal(err)
	}
	for _, key := range []struct {
		id        int64
		status    string
		expiresAt any
		revokedAt any
	}{
		{301, "REVOKED", revokedAt.Add(time.Hour), revokedAt},
		{302, "REVOKED", nil, revokedAt},
		{303, "ACTIVE", revokedAt.Add(time.Hour), nil},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO principal_access_key
			(id,principal_id,key_hash,masked_key,name,status,expires_at,revoked_at,created_by,updated_by,created_at,updated_at)
			VALUES ($1,30,$2,'test****','Test Key',$3,$4,$5,'system','system',$6,$6)`,
			key.id, bytes.Repeat([]byte{byte(key.id)}, 32), key.status, key.expiresAt, key.revokedAt, revokedAt); err != nil {
			t.Fatal(err)
		}
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(ctx, `SELECT id,expires_at FROM principal_access_key WHERE principal_id=30 ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for _, want := range []struct {
		id int64
		at time.Time
	}{
		{301, revokedAt}, {302, revokedAt}, {303, revokedAt.Add(time.Hour)},
	} {
		if !rows.Next() {
			t.Fatalf("missing key %d", want.id)
		}
		var id int64
		var expiresAt time.Time
		if err := rows.Scan(&id, &expiresAt); err != nil || id != want.id || !expiresAt.Equal(want.at) {
			t.Fatalf("key %d expiry = %s, error = %v; want %s", id, expiresAt, err, want.at)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}
