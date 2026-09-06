package management

import (
	"strings"
	"testing"
)

func TestModelInputValidation(t *testing.T) {
	valid := ModelInput{Code: "official-model-v1", Name: "官方模型", InputModalities: []string{"TEXT", "IMAGE"}, OutputModalities: []string{"TEXT"}}
	if !validModelInput(valid) {
		t.Fatal("valid model rejected")
	}
	for _, values := range [][]string{nil, {}, {"TEXT", "TEXT"}, {"text"}, {"UNKNOWN"}, {"TEXT", ""}} {
		for _, input := range []bool{true, false} {
			model := valid
			if input {
				model.InputModalities = values
			} else {
				model.OutputModalities = values
			}
			if validModelInput(model) {
				t.Errorf("invalid modalities accepted: %v", values)
			}
		}
	}
	for _, mutate := range []func(*ModelInput){
		func(m *ModelInput) { m.Code = " " },
		func(m *ModelInput) { m.Name = strings.Repeat("中", 43) },
		func(m *ModelInput) { m.Remark = strings.Repeat("中", 667) },
		func(m *ModelInput) { m.Remark = "\x00" },
	} {
		model := valid
		mutate(&model)
		if validModelInput(model) {
			t.Errorf("invalid model accepted: %+v", model)
		}
	}
}
