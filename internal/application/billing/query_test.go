package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
)

func TestDocumentQueriesValidateActorWindowAndFilters(t *testing.T) {
	service := New(&billingStoreStub{}, &billingIDs{})
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	valid := DocumentFilter{From: from, To: from.AddDate(0, 1, 0), Limit: 50}

	if _, err := service.Documents(context.Background(), admin.Identity{}, valid); !errors.Is(err, appsec.ErrUnauthenticated) {
		t.Fatalf("expected unauthenticated, got %v", err)
	}
	invalid := valid
	invalid.Currency = "EUR"
	if _, err := service.Documents(context.Background(), admin.Identity{ID: 1}, invalid); !errors.Is(err, appsec.ErrInvalidArgument) {
		t.Fatalf("expected invalid currency, got %v", err)
	}
	invalid = valid
	invalid.To = invalid.From.Add(367 * 24 * time.Hour)
	if _, err := service.Documents(context.Background(), admin.Identity{ID: 1}, invalid); !errors.Is(err, appsec.ErrInvalidArgument) {
		t.Fatalf("expected invalid window, got %v", err)
	}
	if _, err := service.Document(context.Background(), admin.Identity{ID: 1}, 0); !errors.Is(err, appsec.ErrInvalidArgument) {
		t.Fatalf("expected invalid id, got %v", err)
	}
}

func TestUnratedUsageValidatesReason(t *testing.T) {
	service := New(&billingStoreStub{}, &billingIDs{})
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	filter := UnratedUsageFilter{
		From: from, To: from.AddDate(0, 1, 0), Reason: "FAILED", Limit: 50,
	}
	if _, err := service.Unrated(context.Background(), admin.Identity{ID: 1}, filter); !errors.Is(err, appsec.ErrInvalidArgument) {
		t.Fatalf("expected invalid reason, got %v", err)
	}
	filter.Reason = "PENDING_RATING"
	if _, err := service.Unrated(context.Background(), admin.Identity{ID: 1}, filter); err != nil {
		t.Fatal(err)
	}
}
