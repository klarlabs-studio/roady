package application

import (
	"fmt"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/domain/team"
)

// teamLoader is the narrow slice of the repository needed for role checks.
// It is asserted optionally rather than added to WorkspaceRepository so that
// repositories and test doubles which do not carry team config keep working.
type teamLoader interface {
	LoadTeam() (*team.TeamConfig, error)
}

// ValidateActorCanTransition enforces .roady/team.yaml roles when the policy
// opts in via enforce_team_roles.
//
// Enforcement is deliberately narrow: only an actor *listed* in team.yaml is
// checked. An unlisted actor passes, because team.yaml has always been a
// partial roster rather than an access-control list, and treating absence as
// denial would lock out every existing project the moment the flag is set.
// The rule it does enforce is the one people actually want: someone recorded
// as a viewer cannot move tasks.
func (s *PolicyService) ValidateActorCanTransition(actor string) error {
	cfg, err := s.repo.LoadPolicy()
	if err != nil || cfg == nil || !cfg.EnforceTeamRoles {
		return nil
	}

	actor = strings.TrimSpace(actor)
	if actor == "" {
		return nil
	}

	loader, ok := s.repo.(teamLoader)
	if !ok {
		return nil
	}

	teamCfg, err := loader.LoadTeam()
	if err != nil || teamCfg == nil {
		return nil
	}

	member := findMemberInsensitive(teamCfg, actor)
	if member == nil {
		return nil
	}

	if !member.Role.CanTransitionTasks() {
		return fmt.Errorf("%s has role %q, which cannot transition tasks; an admin or member must do it, or change the role with 'roady team add %s member'",
			member.Name, member.Role, member.Name)
	}

	return nil
}

// findMemberInsensitive matches a roster entry without requiring the actor
// string to match the recorded casing — actors arrive from git config, USER,
// and MCP clients, none of which agree on capitalisation.
func findMemberInsensitive(cfg *team.TeamConfig, actor string) *team.Member {
	want := strings.ToLower(strings.TrimSpace(actor))
	for i, m := range cfg.Members {
		if strings.ToLower(strings.TrimSpace(m.Name)) == want {
			return &cfg.Members[i]
		}
	}
	return nil
}
