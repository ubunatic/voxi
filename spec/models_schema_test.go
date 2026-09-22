package spec

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestModelsMatchSchema closes the gap left by issue 134 M2: nothing
// exercised spec/schemas/models.schema.json against spec/models.yaml, so a
// schema that drifted from the YAML (an unknown/misspelled field on either
// side) was caught by nothing.
//
// voxi's convention (docs/Go.md) is to avoid dependencies, and go.mod has no
// JSON-schema validation library. Rather than add one just for this check,
// this test enforces the one property that actually matters for catching
// drift without a schema engine: the set of per-model field names used
// anywhere in models.yaml must be a subset of the schema's declared
// per-model properties (schema.additionalProperties: false already makes
// this true operationally -- a real JSON-schema validator would reject an
// extra field the same way), and the schema's declared properties must in
// turn be a subset of the fields the Go Model struct actually knows about
// (via its yaml tags). Together these two subset checks mean: add a field to
// models.yaml the schema doesn't declare -> fails; add a property to the
// schema with no Go field behind it -> fails.
func TestModelsMatchSchema(t *testing.T) {
	// Fields actually used by any model entry in models.yaml.
	var raw struct {
		Models map[string]map[string]any `yaml:"models"`
	}
	if err := yaml.Unmarshal(modelsYAML, &raw); err != nil {
		t.Fatalf("parse models.yaml: %v", err)
	}
	usedFields := make(map[string]bool)
	for _, m := range raw.Models {
		for field := range m {
			usedFields[field] = true
		}
	}

	// Properties declared by the schema for a model entry.
	schemaData, err := os.ReadFile("schemas/models.schema.json")
	if err != nil {
		t.Fatalf("read models.schema.json: %v", err)
	}
	var schema struct {
		Properties struct {
			Models struct {
				AdditionalProperties struct {
					Properties map[string]json.RawMessage `json:"properties"`
				} `json:"additionalProperties"`
			} `json:"models"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schemaData, &schema); err != nil {
		t.Fatalf("parse models.schema.json: %v", err)
	}
	schemaFields := schema.Properties.Models.AdditionalProperties.Properties
	if len(schemaFields) == 0 {
		t.Fatal("models.schema.json declares no per-model properties; schema path assumption is stale")
	}

	// Fields the Go Model struct actually binds, from its yaml tags.
	knownFields := make(map[string]bool)
	modelType := reflect.TypeOf(Model{})
	for i := 0; i < modelType.NumField(); i++ {
		tag := modelType.Field(i).Tag.Get("yaml")
		if tag != "" {
			knownFields[tag] = true
		}
	}

	for field := range usedFields {
		if _, ok := schemaFields[field]; !ok {
			t.Errorf("models.yaml uses field %q that models.schema.json does not declare", field)
		}
	}
	for field := range schemaFields {
		if !knownFields[field] {
			t.Errorf("models.schema.json declares property %q that the Go Model struct (spec/models.go) does not bind", field)
		}
	}
}
