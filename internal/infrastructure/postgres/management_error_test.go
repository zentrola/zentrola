package postgres

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
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
