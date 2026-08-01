package provenance

import "testing"

func envFrom(pairs map[string]string) func(string) string {
	return func(k string) string { return pairs[k] }
}

func TestResolverSessionID(t *testing.T) {
	t.Run("uses ROADY_SESSION_ID when set", func(t *testing.T) {
		r := NewResolver(SurfaceCLI, envFrom(map[string]string{EnvSessionID: "run-42"}), func() string { return "generated" })

		if got := r.Resolve().SessionID; got != "run-42" {
			t.Errorf("SessionID = %q, want run-42", got)
		}
	})

	t.Run("mints one when unset", func(t *testing.T) {
		r := NewResolver(SurfaceCLI, envFrom(nil), func() string { return "generated-id" })

		if got := r.Resolve().SessionID; got != "generated-id" {
			t.Errorf("SessionID = %q, want generated-id", got)
		}
	})

	t.Run("mints only once per resolver", func(t *testing.T) {
		calls := 0
		r := NewResolver(SurfaceMCP, envFrom(nil), func() string {
			calls++
			return "id"
		})

		// A long-lived MCP process must report one session across every
		// tool call, otherwise "everything session X did" is unanswerable.
		for range 5 {
			r.Resolve()
		}

		if calls != 1 {
			t.Errorf("expected the session ID to be minted once, got %d calls", calls)
		}
	})

	t.Run("whitespace-only env is treated as unset", func(t *testing.T) {
		r := NewResolver(SurfaceCLI, envFrom(map[string]string{EnvSessionID: "   "}), func() string { return "generated" })

		if got := r.Resolve().SessionID; got != "generated" {
			t.Errorf("SessionID = %q, want generated", got)
		}
	})
}

func TestResolverAgent(t *testing.T) {
	tests := []struct {
		name    string
		surface Surface
		env     map[string]string
		want    string
	}{
		{
			name:    "explicit ROADY_AGENT wins",
			surface: SurfaceMCP,
			env:     map[string]string{EnvAgent: "my-bot", "CLAUDECODE": "1"},
			want:    "my-bot",
		},
		{
			name:    "detects claude code",
			surface: SurfaceMCP,
			env:     map[string]string{"CLAUDECODE": "1"},
			want:    "claude-code",
		},
		{
			name:    "detects cursor",
			surface: SurfaceMCP,
			env:     map[string]string{"CURSOR_TRACE_ID": "abc"},
			want:    "cursor",
		},
		{
			name:    "detects codex",
			surface: SurfaceMCP,
			env:     map[string]string{"CODEX_SANDBOX": "seatbelt"},
			want:    "codex",
		},
		{
			name:    "cli surface defaults to cli",
			surface: SurfaceCLI,
			env:     nil,
			want:    "cli",
		},
		{
			name:    "mcp surface with nothing detectable stays empty",
			surface: SurfaceMCP,
			env:     nil,
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewResolver(tt.surface, envFrom(tt.env), func() string { return "id" })

			if got := r.Resolve().Agent; got != tt.want {
				t.Errorf("Agent = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestContextApply(t *testing.T) {
	t.Run("stamps onto nil metadata", func(t *testing.T) {
		ctx := Context{SessionID: "s1", Agent: "claude-code", Surface: SurfaceMCP}

		got := ctx.Apply(nil)

		if got[KeySessionID] != "s1" || got[KeyAgent] != "claude-code" || got[KeySurface] != "mcp" {
			t.Errorf("unexpected metadata: %+v", got)
		}
	})

	t.Run("never overwrites a caller-supplied value", func(t *testing.T) {
		ctx := Context{SessionID: "ambient", Agent: "ambient-agent", Surface: SurfaceMCP}
		metadata := map[string]any{KeySessionID: "explicit", "task_id": "t1"}

		got := ctx.Apply(metadata)

		// An agent forwarding the identity of the run that spawned it knows
		// better than the ambient process does.
		if got[KeySessionID] != "explicit" {
			t.Errorf("SessionID = %v, want explicit", got[KeySessionID])
		}
		if got[KeyAgent] != "ambient-agent" {
			t.Errorf("absent keys should still be filled, got %v", got[KeyAgent])
		}
		if got["task_id"] != "t1" {
			t.Error("existing unrelated metadata must survive")
		}
	})

	t.Run("empty values are not stamped", func(t *testing.T) {
		got := Unknown().Apply(nil)

		if len(got) != 0 {
			t.Errorf("expected no keys for an unknown context, got %+v", got)
		}
	})
}

func TestFromMetadata(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]any
		want     Context
	}{
		{
			name:     "round-trips",
			metadata: map[string]any{KeySessionID: "s1", KeyAgent: "codex", KeySurface: "mcp"},
			want:     Context{SessionID: "s1", Agent: "codex", Surface: SurfaceMCP},
		},
		{
			name:     "nil metadata is unknown",
			metadata: nil,
			want:     Unknown(),
		},
		{
			name:     "missing surface defaults to unknown",
			metadata: map[string]any{KeySessionID: "s1"},
			want:     Context{SessionID: "s1", Surface: SurfaceUnknown},
		},
		{
			name:     "non-string values ignored",
			metadata: map[string]any{KeySessionID: 42, KeyAgent: nil},
			want:     Unknown(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FromMetadata(tt.metadata)
			if got != tt.want {
				t.Errorf("FromMetadata() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestContextMatches(t *testing.T) {
	ctx := Context{SessionID: "sess-1", Agent: "claude-code", Surface: SurfaceMCP}

	tests := []struct {
		name    string
		agent   string
		session string
		want    bool
	}{
		{name: "no filter matches", want: true},
		{name: "agent matches", agent: "claude-code", want: true},
		{name: "agent case-insensitive", agent: "Claude-Code", want: true},
		{name: "agent mismatch", agent: "codex", want: false},
		{name: "session matches", session: "sess-1", want: true},
		{name: "session mismatch", session: "sess-2", want: false},
		{name: "both must match", agent: "claude-code", session: "sess-2", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ctx.Matches(tt.agent, tt.session); got != tt.want {
				t.Errorf("Matches(%q, %q) = %v, want %v", tt.agent, tt.session, got, tt.want)
			}
		})
	}
}

func TestContextIsZero(t *testing.T) {
	if !Unknown().IsZero() {
		t.Error("Unknown() should be zero")
	}
	if (Context{Agent: "cli"}).IsZero() {
		t.Error("a context with an agent is not zero")
	}
}

func TestWithSession(t *testing.T) {
	base := Context{SessionID: "ambient", Agent: "ambient", Surface: SurfaceMCP}

	got := base.WithSession("explicit", "codex")
	if got.SessionID != "explicit" || got.Agent != "codex" {
		t.Errorf("WithSession did not override: %+v", got)
	}

	unchanged := base.WithSession("", "  ")
	if unchanged.SessionID != "ambient" || unchanged.Agent != "ambient" {
		t.Errorf("empty overrides must be ignored: %+v", unchanged)
	}
}
