package postgres

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
	appsec "github.com/zentrola/zentrola/internal/application/security"
)

func TestManagementErrorIdentifiesDuplicateSubscriptionAccount(t *testing.T) {
	err := managementError(&pgconn.PgError{
		Code:           "23505",
		ConstraintName: "uk_provider_credential_subscription_account",
	})
	if !errors.Is(err, mgmt.ErrSubscriptionAccountExists) {
		t.Fatalf("error=%v; want ErrSubscriptionAccountExists", err)
	}
}

func TestManagementStoreLogsUnexpectedDatabaseError(t *testing.T) {
	var output bytes.Buffer
	store := &ManagementStore{logger: slog.New(slog.NewJSONHandler(&output, nil))}
	cause := errors.New("connection reset")
	err := store.managementError(context.Background(), "commit_transaction", cause)
	if !errors.Is(err, appsec.ErrUnavailable) {
		t.Fatalf("error=%v; want ErrUnavailable", err)
	}
	logLine := output.String()
	if !strings.Contains(logLine, `"operation":"commit_transaction"`) || !strings.Contains(logLine, "connection reset") {
		t.Fatalf("unexpected diagnostic log: %s", logLine)
	}
}

func TestManagementStoreDoesNotLogKnownApplicationError(t *testing.T) {
	var output bytes.Buffer
	store := &ManagementStore{logger: slog.New(slog.NewJSONHandler(&output, nil))}
	err := store.managementError(context.Background(), "execute_callback", mgmt.ErrConflict)
	if !errors.Is(err, mgmt.ErrConflict) || output.Len() != 0 {
		t.Fatalf("error=%v log=%q", err, output.String())
	}
}
