package cli

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/application"
	"gopkg.in/yaml.v3"
)

// captureKinds names each capture item type the way a person writing the
// document thinks of it, with the fields it accepts.
var captureKinds = map[string]struct {
	noun   string
	fields []string
}{
	"application.CaptureDoc":         {"the document", yamlFields(application.CaptureDoc{})},
	"application.CaptureDecision":    {"a decision", yamlFields(application.CaptureDecision{})},
	"application.CaptureGoal":        {"a goal", yamlFields(application.CaptureGoal{})},
	"application.CaptureFeature":     {"a feature", yamlFields(application.CaptureFeature{})},
	"application.CaptureRequirement": {"a requirement", yamlFields(application.CaptureRequirement{})},
	"application.CaptureTask":        {"a task", yamlFields(application.CaptureTask{})},
}

// yamlFields lists the YAML keys a struct accepts, in declaration order.
func yamlFields(v any) []string {
	t := reflect.TypeOf(v)
	var out []string
	for i := 0; i < t.NumField(); i++ {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("yaml"), ",")
		if name != "" && name != "-" {
			out = append(out, name)
		}
	}
	return out
}

var unknownField = regexp.MustCompile(`^(line \d+): field (\S+) not found in type (\S+)$`)

// explainCaptureError rewrites yaml's unknown-field errors, which name Go
// types ("field feature not found in type application.CaptureTask"), into
// what the writer needs: which item, the nearest field it has, and the rest.
func explainCaptureError(err error) error {
	te, ok := err.(*yaml.TypeError)
	if !ok {
		return err
	}
	lines := make([]string, 0, len(te.Errors))
	for _, e := range te.Errors {
		m := unknownField.FindStringSubmatch(e)
		if m == nil {
			lines = append(lines, e)
			continue
		}
		kind, known := captureKinds[strings.TrimPrefix(m[3], "*")]
		if !known {
			lines = append(lines, e)
			continue
		}
		line := fmt.Sprintf("%s: %s has no field %q", m[1], kind.noun, m[2])
		if near := nearestField(m[2], kind.fields); near != "" {
			line += fmt.Sprintf(" (did you mean %q?)", near)
		}
		line += "; it takes " + strings.Join(kind.fields, ", ")
		lines = append(lines, line)
	}
	return fmt.Errorf("%s", strings.Join(lines, "\n  "))
}

// nearestField suggests the field a misspelt key most likely meant: one it is
// a prefix or suffix of (feature → feature_id), else the closest by edit
// distance when that is small.
func nearestField(name string, fields []string) string {
	for _, f := range fields {
		if strings.HasPrefix(f, name+"_") || strings.HasSuffix(f, "_"+name) {
			return f
		}
	}
	best, bestDist := "", 3
	sorted := append([]string(nil), fields...)
	sort.Strings(sorted)
	for _, f := range sorted {
		if d := editDistance(name, f); d < bestDist {
			best, bestDist = f, d
		}
	}
	return best
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}
