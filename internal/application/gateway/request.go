package gateway

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"
)

type Parsed struct {
	Model                string
	Stream               bool
	modelStart, modelEnd int
}

func validModel(model string) bool {
	return len(model) > 0 && len(model) <= 128 && strings.TrimSpace(model) == model && !strings.ContainsAny(model, "\x00\r\n\t ")
}

// Parse 仅检查路由必需字段。其余 JSON 原文保留，避免工具输入数字或未知字段被重新编码。
func Parse(body []byte) (Parsed, error) {
	return parse(body, false)
}
func ParseOpenAI(body []byte) (Parsed, error) { return parse(body, true) }
func parse(body []byte, nullableStream bool) (Parsed, error) {
	var result Parsed
	if !utf8.Valid(body) {
		return result, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(body))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return result, ErrInvalid
	}
	seen := map[string]bool{}
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return result, ErrInvalid
		}
		key, ok := token.(string)
		if !ok || seen[key] {
			return result, ErrInvalid
		}
		seen[key] = true
		var raw json.RawMessage
		if err := d.Decode(&raw); err != nil {
			return result, ErrInvalid
		}
		switch key {
		case "model":
			if err := json.Unmarshal(raw, &result.Model); err != nil || !validModel(result.Model) {
				return result, ErrInvalid
			}
			result.modelEnd = int(d.InputOffset())
			result.modelStart = result.modelEnd - len(raw)
		case "stream":
			if nullableStream && bytes.Equal(raw, []byte("null")) {
				continue
			}
			if !bytes.Equal(raw, []byte("true")) && !bytes.Equal(raw, []byte("false")) {
				return result, ErrInvalid
			}
			result.Stream = bytes.Equal(raw, []byte("true"))
		}
	}
	if _, err := d.Token(); err != nil {
		return result, ErrInvalid
	}
	var trailing any
	if err := d.Decode(&trailing); err != io.EOF {
		return result, ErrInvalid
	}
	if !seen["model"] {
		return result, ErrInvalid
	}
	return result, nil
}
func (p Parsed) Rewrite(body []byte, model string) []byte {
	encoded, _ := json.Marshal(model)
	out := make([]byte, 0, len(body)+len(encoded))
	out = append(out, body[:p.modelStart]...)
	out = append(out, encoded...)
	return append(out, body[p.modelEnd:]...)
}
