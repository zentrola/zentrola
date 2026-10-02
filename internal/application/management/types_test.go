package management

import (
	"errors"
	"fmt"
	"testing"
)

func TestPublicErrorRecognizesWrappedErrors(t *testing.T) {
	for _, expected := range PublicErrors() {
		actual, ok := PublicError(fmt.Errorf("adapter context: %w", expected))
		if !ok || !errors.Is(actual, expected) {
			t.Fatalf("failed to normalize %q: actual=%v ok=%v", expected, actual, ok)
		}
	}
	if _, ok := PublicError(errors.New("internal")); ok {
		t.Fatal("internal error must not become public")
	}
}
