package org

import (
	"strings"
	"testing"
)

func TestMemberSetUsableSkipsUnresolvable(t *testing.T) {
	set := MemberSet{Members: []Member{
		{Declared: "./api", Path: "/w/api", RoadyDir: "/w/api/.roady"},
		{Declared: "./gone", Problem: "no such directory"},
		{Declared: "./web", Path: "/w/web", RoadyDir: "/w/web/.roady"},
	}}

	usable := set.Usable()
	if len(usable) != 2 {
		t.Fatalf("got %d usable members, want 2", len(usable))
	}
	for _, m := range usable {
		if !m.OK() {
			t.Errorf("member %q is not OK but was returned as usable", m.Declared)
		}
	}
}

// A declared member that cannot be resolved must be reported. Dropping it
// silently is how a workspace ends up reporting healthy progress across
// repositories it is not actually looking at.
func TestMemberSetReportsProblems(t *testing.T) {
	set := MemberSet{Members: []Member{
		{Declared: "./api", Path: "/w/api", RoadyDir: "/w/api/.roady"},
		{Declared: "./gone", Problem: "no such directory"},
	}}

	problems := set.Problems()
	if len(problems) != 1 {
		t.Fatalf("got %d problems, want 1: %v", len(problems), problems)
	}
	if !strings.Contains(problems[0], "./gone") {
		t.Errorf("problem does not name the member: %q", problems[0])
	}
	if !strings.Contains(problems[0], "no such directory") {
		t.Errorf("problem does not give the reason: %q", problems[0])
	}
	if !set.HasProblems() {
		t.Error("HasProblems() = false with an unresolvable member")
	}
}

func TestMemberSetDeclaredReportsWhetherMembershipIsExplicit(t *testing.T) {
	declared := MemberSet{Declared: true, Members: []Member{{Declared: "./api", Path: "/w/api"}}}
	if !declared.IsDeclared() {
		t.Error("IsDeclared() = false for a set built from org.yaml")
	}

	// Discovery found the projects; nobody declared them. The distinction
	// matters because a declared set is authoritative and a discovered one is
	// a best guess.
	discovered := MemberSet{Members: []Member{{Declared: "./api", Path: "/w/api"}}}
	if discovered.IsDeclared() {
		t.Error("IsDeclared() = true for a discovered set")
	}
}

func TestMemberOK(t *testing.T) {
	if (Member{Path: "/w/api", RoadyDir: "/w/api/.roady"}).OK() != true {
		t.Error("a resolved member reports not OK")
	}
	if (Member{Declared: "./x", Problem: "boom"}).OK() != false {
		t.Error("a member with a problem reports OK")
	}
	if (Member{Declared: "./x"}).OK() != false {
		t.Error("a member with no resolved path reports OK")
	}
}

func TestMemberSetNamesAreStable(t *testing.T) {
	set := MemberSet{Members: []Member{
		{Declared: "./services/api", Path: "/w/services/api", RoadyDir: "/w/services/api/.roady"},
		{Declared: "../shared/web", Path: "/shared/web", RoadyDir: "/shared/web/.roady"},
	}}

	names := set.Names()
	if len(names) != 2 {
		t.Fatalf("got %d names, want 2", len(names))
	}
	// The base name identifies a member in a report; the declared path is
	// how the operator wrote it and is not always readable.
	if names[0] != "api" || names[1] != "web" {
		t.Errorf("names = %v, want [api web]", names)
	}
}
