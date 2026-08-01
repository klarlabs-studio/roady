package application

import (
	"errors"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/policy"
	"github.com/felixgeelhaar/roady/pkg/domain/team"
)

var errBoom = errors.New("boom")

// roleRepo is a minimal repository double implementing only what the role
// check touches, plus the teamLoader assertion.
type roleRepo struct {
	domain.WorkspaceRepository
	policyCfg *policy.PolicyConfig
	policyErr error
	teamCfg   *team.TeamConfig
	teamErr   error
}

func (r *roleRepo) LoadPolicy() (*policy.PolicyConfig, error) { return r.policyCfg, r.policyErr }
func (r *roleRepo) LoadTeam() (*team.TeamConfig, error)       { return r.teamCfg, r.teamErr }

func roster() *team.TeamConfig {
	return &team.TeamConfig{Members: []team.Member{
		{Name: "ada", Role: team.RoleAdmin},
		{Name: "Mel", Role: team.RoleMember},
		{Name: "vic", Role: team.RoleViewer},
	}}
}

func TestValidateActorCanTransition(t *testing.T) {
	enforcing := &policy.PolicyConfig{EnforceTeamRoles: true}

	tests := []struct {
		name      string
		repo      *roleRepo
		actor     string
		wantErr   bool
		errSubstr string
	}{
		{
			name:  "admin may transition",
			repo:  &roleRepo{policyCfg: enforcing, teamCfg: roster()},
			actor: "ada",
		},
		{
			name:  "member may transition",
			repo:  &roleRepo{policyCfg: enforcing, teamCfg: roster()},
			actor: "Mel",
		},
		{
			name:      "viewer may not transition",
			repo:      &roleRepo{policyCfg: enforcing, teamCfg: roster()},
			actor:     "vic",
			wantErr:   true,
			errSubstr: "cannot transition tasks",
		},
		{
			name:      "viewer match is case-insensitive",
			repo:      &roleRepo{policyCfg: enforcing, teamCfg: roster()},
			actor:     "VIC",
			wantErr:   true,
			errSubstr: "cannot transition tasks",
		},
		{
			name:  "unlisted actor is allowed - team.yaml is a roster, not an ACL",
			repo:  &roleRepo{policyCfg: enforcing, teamCfg: roster()},
			actor: "stranger",
		},
		{
			name:  "empty actor is allowed",
			repo:  &roleRepo{policyCfg: enforcing, teamCfg: roster()},
			actor: "",
		},
		{
			name:  "viewer passes when enforcement is off",
			repo:  &roleRepo{policyCfg: &policy.PolicyConfig{}, teamCfg: roster()},
			actor: "vic",
		},
		{
			name:  "no policy config means no enforcement",
			repo:  &roleRepo{teamCfg: roster()},
			actor: "vic",
		},
		{
			name:  "unreadable team config does not block work",
			repo:  &roleRepo{policyCfg: enforcing, teamErr: errBoom},
			actor: "vic",
		},
		{
			name:  "unreadable policy does not block work",
			repo:  &roleRepo{policyErr: errBoom, teamCfg: roster()},
			actor: "vic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewPolicyService(tt.repo)

			err := svc.ValidateActorCanTransition(tt.actor)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error for actor %q", tt.actor)
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q should contain %q", err, tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error for actor %q: %v", tt.actor, err)
			}
		})
	}
}

// repoWithoutTeam does not implement teamLoader, exercising the optional
// interface assertion.
type repoWithoutTeam struct {
	domain.WorkspaceRepository
	policyCfg *policy.PolicyConfig
}

func (r *repoWithoutTeam) LoadPolicy() (*policy.PolicyConfig, error) { return r.policyCfg, nil }

func TestValidateActorCanTransitionWithoutTeamSupport(t *testing.T) {
	svc := NewPolicyService(&repoWithoutTeam{policyCfg: &policy.PolicyConfig{EnforceTeamRoles: true}})

	if err := svc.ValidateActorCanTransition("anyone"); err != nil {
		t.Errorf("a repository without team support must not block transitions: %v", err)
	}
}

func TestFindMemberInsensitive(t *testing.T) {
	cfg := roster()

	tests := []struct {
		actor string
		want  string
	}{
		{actor: "ada", want: "ada"},
		{actor: "ADA", want: "ada"},
		{actor: "  mel  ", want: "Mel"},
		{actor: "nobody", want: ""},
	}

	for _, tt := range tests {
		got := findMemberInsensitive(cfg, tt.actor)
		if tt.want == "" {
			if got != nil {
				t.Errorf("findMemberInsensitive(%q) = %v, want nil", tt.actor, got.Name)
			}
			continue
		}
		if got == nil || got.Name != tt.want {
			t.Errorf("findMemberInsensitive(%q) = %v, want %q", tt.actor, got, tt.want)
		}
	}
}
