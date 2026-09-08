package anthropic

import (
	"strings"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
)

const (
	deepSeekAdvisorTool = "advisor_20260301"
	deepSeekAdvisorBeta = "advisor-tool-2026-03-01"
)

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
