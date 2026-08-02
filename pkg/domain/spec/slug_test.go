package spec

import (
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		// The cases from issue #74, verbatim from a real spec.
		{"em dash", "Phase A — Pilot Finalization (2 weeks)", "phase-a-pilot-finalization-2-weeks"},
		{"slashes", "Wearable Ingestion (Garmin / Polar / HRV4Training)", "wearable-ingestion-garmin-polar-hrv4training"},
		{"plus", "P1 — /register Privacy Footer + Datenschutz Link", "p1-register-privacy-footer-datenschutz-link"},
		{"parens", "Federation Export Pack (DOSB / IAT)", "federation-export-pack-dosb-iat"},

		{"plain", "User Authentication", "user-authentication"},
		{"already a slug", "user-authentication", "user-authentication"},
		{"collapses runs", "A  --  B", "a-b"},
		{"trims separators", "  -Hello-  ", "hello"},
		{"keeps digits", "OAuth 2.0 Support", "oauth-2-0-support"},
		{"folds accents", "Datenschutz für Prüfung", "datenschutz-fur-prufung"},
		{"folds more accents", "Café Naïve Señor", "cafe-naive-senor"},
		{"keeps non-latin letters", "認証システム", "認証システム"},
		{"underscores are separators", "some_thing", "some-thing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Slugify(tt.title); got != tt.want {
				t.Errorf("Slugify(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

// A slug is used as a path component, a URL segment and a shell argument.
// Characters that break any of those must never survive.
func TestSlugifyNeverEmitsUnsafeCharacters(t *testing.T) {
	unsafe := `/\:*?"<>|+()[]{}&;$#%'` + "`" + ` ` + "\t\n"

	titles := []string{
		"Wearable Ingestion (Garmin / Polar)",
		"a/b/../../etc/passwd",
		"C:\\Windows\\System32",
		"rm -rf $HOME; echo 'pwned'",
		"100% & <script>alert(1)</script>",
		"tabs\tand\nnewlines",
	}

	for _, title := range titles {
		got := Slugify(title)
		if strings.ContainsAny(got, unsafe) {
			t.Errorf("Slugify(%q) = %q, which contains an unsafe character", title, got)
		}
	}
}

// An id with an embedded slash would silently nest a path under
// .roady/projects/<name>/, so path traversal must not survive slugification.
func TestSlugifyDefeatsPathTraversal(t *testing.T) {
	got := Slugify("../../etc/passwd")
	if strings.Contains(got, "/") || strings.Contains(got, "..") {
		t.Errorf("Slugify traversal = %q, want no path separators or parent references", got)
	}
	if got != "etc-passwd" {
		t.Errorf("Slugify(\"../../etc/passwd\") = %q, want %q", got, "etc-passwd")
	}
}

// A title made entirely of punctuation still has to yield a usable id,
// because the alternative is a feature with an empty id colliding with every
// other such feature.
func TestSlugifyFallsBackWhenNothingSurvives(t *testing.T) {
	for _, title := range []string{"???", "", "   ", "!@#$%^&*()"} {
		got := Slugify(title)
		if got == "" {
			t.Fatalf("Slugify(%q) = %q, want a non-empty fallback", title, got)
		}
		if !strings.HasPrefix(got, "id-") {
			t.Errorf("Slugify(%q) = %q, want an id- prefixed fallback", title, got)
		}
	}

	// The fallback is derived from the title, so distinct titles stay distinct
	// and the same title is stable across runs.
	question, bang := Slugify("???"), Slugify("!!!")
	if question == bang {
		t.Error("distinct unusable titles produced the same fallback id")
	}
	if again := Slugify("???"); again != question {
		t.Errorf("fallback id is not stable: %q then %q", question, again)
	}
}

// Ids end up in filenames; an unbounded one is a portability problem.
func TestSlugifyIsLengthBounded(t *testing.T) {
	long := strings.Repeat("very long feature title ", 40)
	got := Slugify(long)

	if len(got) > slugMaxLen {
		t.Errorf("Slugify produced %d bytes, want at most %d", len(got), slugMaxLen)
	}
	if strings.HasSuffix(got, "-") {
		t.Errorf("Slugify(%q…) = %q, want no trailing separator after truncation", long[:20], got)
	}
}
