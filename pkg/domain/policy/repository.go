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
	// VerifyRequiresEvidence makes verified mean proven: a task can only be
	// verified with a passing acceptance check and a linked commit, unless a
	// person overrides it on the record. Off for existing projects so an
	// upgrade does not refuse work that was verified the old way; `roady init`
	// turns it on for new ones.
	VerifyRequiresEvidence bool `yaml:"verify_requires_evidence"`
}

// Repository handles persistence of policy configurations.
type Repository interface {
	Save(cfg *PolicyConfig) error
	Load() (*PolicyConfig, error)
}
