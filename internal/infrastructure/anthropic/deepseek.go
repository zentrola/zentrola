package anthropic

import (
	"encoding/json"
	"strings"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
)

const (
	deepSeekAdvisorTool          = "advisor_20260301"
	deepSeekAdvisorBeta          = "advisor-tool-2026-03-01"
	deepSeekProbeMaxOutputTokens = 32
)

type deepSeekConnectionProbeAdapter struct {
	standard standardConnectionProbeAdapter
}

func (a deepSeekConnectionProbeAdapter) Build(target mgmt.ConnectionTarget, base string) (connectionProbe, bool) {
	probe, ok := a.standard.Build(target, base)
	if !ok {
		return connectionProbe{}, false
	}
	switch target.Protocol {
	case "OPENAI":
		probe.Body = deepSeekOpenAIProbeBody(target.UpstreamModelCode)
	case "ANTHROPIC":
		probe.Body = deepSeekAnthropicProbeBody(target.UpstreamModelCode)
	default:
		return connectionProbe{}, false
	}
	return probe, true
}

func (a deepSeekConnectionProbeAdapter) ValidResponse(data []byte, probe connectionProbe) bool {
	return a.standard.ValidResponse(data, probe)
}

type deepSeekProbeThinking struct {
	Type string `json:"type"`
}

func deepSeekOpenAIProbeBody(model string) []byte {
	body, _ := json.Marshal(struct {
		Model       string                `json:"model"`
		Messages    []probeMessage        `json:"messages"`
		MaxTokens   int                   `json:"max_tokens"`
		Temperature float64               `json:"temperature"`
		Stream      bool                  `json:"stream"`
		Thinking    deepSeekProbeThinking `json:"thinking"`
	}{
		Model: model, Messages: []probeMessage{{Role: "user", Content: "Reply only with OK."}},
		MaxTokens: deepSeekProbeMaxOutputTokens, Thinking: deepSeekProbeThinking{Type: "disabled"},
	})
	return body
}

func deepSeekAnthropicProbeBody(model string) []byte {
	body, _ := json.Marshal(struct {
		Model       string                `json:"model"`
		MaxTokens   int                   `json:"max_tokens"`
		Temperature int                   `json:"temperature"`
		Stream      bool                  `json:"stream"`
		Messages    []probeMessage        `json:"messages"`
		Thinking    deepSeekProbeThinking `json:"thinking"`
	}{
		Model: model, MaxTokens: deepSeekProbeMaxOutputTokens,
		Messages: []probeMessage{{Role: "user", Content: "Reply only with OK."}},
		Thinking: deepSeekProbeThinking{Type: "disabled"},
	})
	return body
}

func adaptDeepSeekRequest(input gw.Request) (gw.Request, int, bool, error) {
	parsed, err := gw.Parse(input.Body)
	if err != nil {
		return input, 0, false, err
	}
	var removedTools int
	input.Body, removedTools, err = parsed.RemoveToolTypes(input.Body, deepSeekAdvisorTool)
	if err != nil {
		return input, 0, false, err
	}
	removedBeta := false
	input.Beta, removedBeta = removeBeta(input.Beta, deepSeekAdvisorBeta)
	if len(input.ProtocolHeaders) > 0 {
		headers := make(map[string][]string, len(input.ProtocolHeaders))
		for name, values := range input.ProtocolHeaders {
			if !strings.EqualFold(name, "anthropic-beta") {
				headers[name] = append([]string(nil), values...)
				continue
			}
			for _, value := range values {
				filtered, removed := removeBeta(value, deepSeekAdvisorBeta)
				removedBeta = removedBeta || removed
				if filtered != "" {
					headers[name] = append(headers[name], filtered)
				}
			}
		}
		input.ProtocolHeaders = headers
	}
	return input, removedTools, removedBeta, nil
}

func removeBeta(value, denied string) (string, bool) {
	if value == "" {
		return "", false
	}
	parts := strings.Split(value, ",")
	kept := make([]string, 0, len(parts))
	removed := false
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == denied {
			removed = true
			continue
		}
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, ","), removed
}
