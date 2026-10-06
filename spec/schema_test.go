package spec

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v3"
)

func TestSchemasValidateYAML(t *testing.T) {
	specs := []struct {
		yamlPath   string
		schemaPath string
	}{
		{"actions.yaml", "schemas/actions.schema.json"},
		{"eager.yaml", "schemas/eager.schema.json"},
		{"models.yaml", "schemas/models.schema.json"},
		{"monitor.yaml", "schemas/monitor.schema.json"},
		{"tts.yaml", "schemas/tts.schema.json"},
	}

	for _, s := range specs {
		t.Run(s.yamlPath, func(t *testing.T) {
			compiler := jsonschema.NewCompiler()
			compiler.Draft = jsonschema.Draft7
			compiler.AssertFormat = true

			schemaBytes, err := os.ReadFile(s.schemaPath)
			if err != nil {
				t.Fatalf("read schema %s: %v", s.schemaPath, err)
			}

			if err := compiler.AddResource(s.schemaPath, bytes.NewReader(schemaBytes)); err != nil {
				t.Fatalf("add resource %s: %v", s.schemaPath, err)
			}

			sch, err := compiler.Compile(s.schemaPath)
			if err != nil {
				t.Fatalf("compile schema %s: %v", s.schemaPath, err)
			}

			yamlBytes, err := os.ReadFile(s.yamlPath)
			if err != nil {
				t.Fatalf("read yaml %s: %v", s.yamlPath, err)
			}

			var yamlObj any
			if err := yaml.Unmarshal(yamlBytes, &yamlObj); err != nil {
				t.Fatalf("unmarshal yaml %s: %v", s.yamlPath, err)
			}

			jsonBytes, err := json.Marshal(yamlObj)
			if err != nil {
				t.Fatalf("marshal yaml obj to json %s: %v", s.yamlPath, err)
			}

			var jsonObj any
			if err := json.Unmarshal(jsonBytes, &jsonObj); err != nil {
				t.Fatalf("unmarshal json %s: %v", s.yamlPath, err)
			}

			if err := sch.Validate(jsonObj); err != nil {
				t.Errorf("schema validation failed for %s against %s:\n%v", s.yamlPath, s.schemaPath, err)
			}
		})
	}
}

func TestAllYAMLHaveSchemas(t *testing.T) {
	matches, err := filepath.Glob("*.yaml")
	if err != nil {
		t.Fatalf("glob *.yaml: %v", err)
	}
	for _, y := range matches {
		base := y[:len(y)-len(".yaml")]
		schemaPath := filepath.Join("schemas", base+".schema.json")
		if _, err := os.Stat(schemaPath); err != nil {
			t.Errorf("yaml spec %s has no matching JSON Schema at %s", y, schemaPath)
		}
	}
}
