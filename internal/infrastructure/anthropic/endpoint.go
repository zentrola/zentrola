package anthropic

import "github.com/zentrola/zentrola/internal/infrastructure/provider"

func allowedBaseURL(raw string) (string, bool) { return provider.BaseURL(raw) }
