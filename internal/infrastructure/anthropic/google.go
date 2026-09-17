package anthropic

import (
	"encoding/json"
	"net/url"
	"strings"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
)

const googleProbeMaxOutputTokens = 128

type googleConnectionProbeAdapter struct{}

type googleProbePart struct {
	Text string `json:"text"`
}

type googleProbeContent struct {
	Role  string            `json:"role"`
	Parts []googleProbePart `json:"parts"`
}

type googleGenerationConfig struct {
	MaxOutputTokens int `json:"maxOutputTokens"`
	Temperature     int `json:"temperature"`
}

func (googleConnectionProbeAdapter) Build(target mgmt.ConnectionTarget, base string) (connectionProbe, bool) {
	model := strings.TrimSpace(target.UpstreamModelCode)
	if target.Protocol != "OPENAI" || model == "" || strings.ContainsAny(model, "/\\") {
		return connectionProbe{}, false
	}
	nativeBase, ok := strings.CutSuffix(strings.TrimSuffix(base, "/"), "/openai")
	if !ok {
		return connectionProbe{}, false
	}
	return connectionProbe{
		URL:          nativeBase + "/models/" + url.PathEscape(model) + ":generateContent",
		Body:         googleProbeBody(),
		APIKeyHeader: "X-Goog-Api-Key",
	}, true
}

func (googleConnectionProbeAdapter) ValidResponse(data []byte, _ connectionProbe) bool {
	var payload struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if json.Unmarshal(data, &payload) != nil {
		return false
	}
	for _, candidate := range payload.Candidates {
		for _, part := range candidate.Content.Parts {
			if strings.TrimSpace(part.Text) != "" {
				return true
			}
		}
	}
	return false
}

func googleProbeBody() []byte {
	body, _ := json.Marshal(struct {
		Contents         []googleProbeContent   `json:"contents"`
		GenerationConfig googleGenerationConfig `json:"generationConfig"`
	}{
		Contents:         []googleProbeContent{{Role: "user", Parts: []googleProbePart{{Text: "Reply only with OK."}}}},
		GenerationConfig: googleGenerationConfig{MaxOutputTokens: googleProbeMaxOutputTokens},
	})
	return body
}
