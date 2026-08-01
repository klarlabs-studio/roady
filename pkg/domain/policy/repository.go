package policy

// PolicyConfig is the serialized representation of policy.yaml
type PolicyConfig struct {
	MaxWIP int `yaml:"max_wip"`
	// MaxWIPPerOwner caps in-progress tasks per person or agent. Zero
	// disables it. A project-wide MaxWIP alone lets one person hold the
	// whole allowance, so this is the limit that makes WIP a coordination
	// signal on a team.
	MaxWIPPerOwner int  `yaml:"max_wip_per_owner"`
	AllowAI        bool `yaml:"allow_ai"`
	TokenLimit     int  `yaml:"token_limit"`
	BudgetHours    int  `yaml:"budget_hours"`
	// EnforceTeamRoles turns .roady/team.yaml from documentation into a
	// guard: when true, an actor listed there must hold a role permitting
	// the operation. Off by default so existing projects keep working.
	EnforceTeamRoles bool `yaml:"enforce_team_roles"`
}

// Repository handles persistence of policy configurations.
type Repository interface {
	Save(cfg *PolicyConfig) error
	Load() (*PolicyConfig, error)
}
