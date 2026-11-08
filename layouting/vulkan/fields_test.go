package vulkan

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var (
	goFieldPattern      = regexp.MustCompile(`(?m)^\s*(node[A-Za-z0-9]+)\s*=\s*([0-9]+)\s*$`)
	shaderDefinePattern = regexp.MustCompile(`(?m)^#define\s+([A-Z0-9_]+)\s+([0-9]+)\s*$`)
)

// TestShaderFieldOffsetsMatch guards the hand-maintained node layout constants
// in fields.go against the copies embedded in shaders/layout.comp.
func TestShaderFieldOffsetsMatch(t *testing.T) {
	fieldsSource, err := os.ReadFile("fields.go")
	if err != nil {
		t.Fatal(err)
	}
	shaderSource, err := os.ReadFile("shaders/layout.comp")
	if err != nil {
		t.Fatal(err)
	}
	shaderValues := map[string]int{}
	for _, match := range shaderDefinePattern.FindAllStringSubmatch(string(shaderSource), -1) {
		value, err := strconv.Atoi(match[2])
		if err != nil {
			continue
		}
		shaderValues[match[1]] = value
	}
	if shaderValues["NODE_FIELD_COUNT"] != nodeFieldCount {
		t.Fatalf("NODE_FIELD_COUNT = %d, want %d", shaderValues["NODE_FIELD_COUNT"], nodeFieldCount)
	}
	count := 0
	for _, match := range goFieldPattern.FindAllStringSubmatch(string(fieldsSource), -1) {
		name := shaderFieldName(match[1])
		value, err := strconv.Atoi(match[2])
		if err != nil {
			t.Fatal(err)
		}
		shaderValue, ok := shaderValues[name]
		if !ok {
			t.Errorf("%s is missing from shaders/layout.comp", name)
			continue
		}
		if shaderValue != value {
			t.Errorf("%s = %d in layout.comp, want %d", name, shaderValue, value)
		}
		count++
	}
	if count == 0 {
		t.Fatal("no node field constants found")
	}
}

func shaderFieldName(goName string) string {
	name := strings.TrimPrefix(goName, "node")
	var builder strings.Builder
	builder.WriteString("F_")
	for index, character := range name {
		if index > 0 && character >= 'A' && character <= 'Z' {
			previous := rune(name[index-1])
			nextLower := index+1 < len(name) && name[index+1] >= 'a' && name[index+1] <= 'z'
			if (previous >= 'a' && previous <= 'z') || (previous >= '0' && previous <= '9') || (previous >= 'A' && previous <= 'Z' && nextLower) {
				builder.WriteByte('_')
			}
		}
		builder.WriteRune(character)
	}
	return strings.ToUpper(builder.String())
}
