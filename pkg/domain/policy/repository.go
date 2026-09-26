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
	// VerifyRequiresEvidence makes verified mean proven: a task can only be
	// verified with a passing acceptance check and a linked commit, unless a
	// person overrides it on the record. Off for existing projects so an
	// upgrade does not refuse work that was verified the old way; `roady init`
	// turns it on for new ones.
	VerifyRequiresEvidence bool `yaml:"verify_requires_evidence"`
	// PlanApproval decides which changes send an approved plan back to
	// pending. "scope" (the default when empty): only changes to intent —
	// features and requirements, including their checks — need re-approval;
	// adding, splitting or editing tasks keeps it. "every_change": any change
	// does, as before.
	PlanApproval string `yaml:"plan_approval,omitempty"`
	// PlanFilesAllow lists project paths (globs, "dir/**" for a subtree) the
	// Claude Code write guard lets through even though they look like plan
	// files (ROADMAP*.md, TODO*.md, plan.md, …). Plans belong in roady; this
	// is for the files a project keeps on purpose.
	PlanFilesAllow []string `yaml:"plan_files_allow,omitempty"`
}

const (
	PlanApprovalScope       = "scope"
	PlanApprovalEveryChange = "every_change"
)

// ApprovalMode returns the effective plan approval mode.
func (c *PolicyConfig) ApprovalMode() string {
	if c == nil || c.PlanApproval == "" {
		return PlanApprovalScope
	}
	return c.PlanApproval
}

// Repository handles persistence of policy configurations.
type Repository interface {
	Save(cfg *PolicyConfig) error
	Load() (*PolicyConfig, error)
}
