package provenance

import "testing"

// A session the caller names spans a conversation and is marked so; a
// minted one is a single command and is not.
func TestSessionGiven(t *testing.T) {
	env := map[string]string{EnvSessionID: "conv-1"}
	given := NewResolver(SurfaceCLI, func(k string) string { return env[k] }, func() string { return "minted" }).Resolve()
	if !given.SessionGiven || given.Apply(nil)[KeySessionGiven] != true {
		t.Errorf("given session not marked: %+v", given)
	}
	minted := NewResolver(SurfaceCLI, func(string) string { return "" }, func() string { return "minted" }).Resolve()
	if minted.SessionGiven {
		t.Error("a minted session is not given")
	}
	if _, ok := minted.Apply(nil)[KeySessionGiven]; ok {
		t.Error("a minted session must not be marked given")
	}
}
