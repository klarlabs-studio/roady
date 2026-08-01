package cli

import "testing"

func TestResolveCurrentOwner(t *testing.T) {
	tests := []struct {
		name      string
		roadyUser string
		osUser    string
		gitName   string
		want      string
	}{
		{
			name:      "ROADY_USER wins over everything",
			roadyUser: "alice",
			osUser:    "root",
			gitName:   "Alice Example",
			want:      "alice",
		},
		{
			name:    "falls back to git user.name",
			osUser:  "root",
			gitName: "Alice Example",
			want:    "Alice Example",
		},
		{
			name:   "falls back to USER when git has no name",
			osUser: "alice",
			want:   "alice",
		},
		{
			name:      "surrounding whitespace trimmed",
			roadyUser: "  alice  ",
			want:      "alice",
		},
		{
			name: "no identity available",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ROADY_USER", tt.roadyUser)
			t.Setenv("USER", tt.osUser)

			got := resolveCurrentOwner(func() string { return tt.gitName })
			if got != tt.want {
				t.Errorf("resolveCurrentOwner() = %q, want %q", got, tt.want)
			}
		})
	}
}
