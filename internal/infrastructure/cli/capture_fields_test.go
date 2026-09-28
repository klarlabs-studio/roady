package cli

import (
	"strings"
	"testing"
)

// Writing `feature:` on a task, as the features list suggests, produced
// "field feature not found in type application.CaptureTask". The error now
// names the item, the field meant, and the fields there are.
func TestCaptureUnknownFieldNamesTheFieldMeant(t *testing.T) {
	_, err := parseCaptureDoc([]byte("tasks:\n  - id: t\n    title: T\n    feature: f\n"))
	if err == nil {
		t.Fatal("an unknown task field was accepted")
	}
	for _, want := range []string{`line 4: a task has no field "feature"`, `did you mean "feature_id"?`, "it takes id, title,", "requirement"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q:\n%v", want, err)
		}
	}
	if strings.Contains(err.Error(), "application.") {
		t.Errorf("error still names Go types:\n%v", err)
	}

	_, err = parseCaptureDoc([]byte("features:\n  - id: f\n    titel: F\n"))
	if err == nil || !strings.Contains(err.Error(), `a feature has no field "titel" (did you mean "title"?)`) {
		t.Errorf("misspelt feature field: %v", err)
	}
}
