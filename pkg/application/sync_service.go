package application

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	domainPlugin "github.com/felixgeelhaar/roady/pkg/domain/plugin"
	"github.com/felixgeelhaar/roady/pkg/plugin"
)

// PluginConfigRepository provides access to plugin configurations
type PluginConfigRepository interface {
	LoadPluginConfigs() (*domainPlugin.PluginConfigs, error)
	GetPluginConfig(name string) (*domainPlugin.PluginConfig, error)
	SetPluginConfig(name string, cfg domainPlugin.PluginConfig) error
}

type SyncService struct {
	repo       domain.WorkspaceRepository
	pluginRepo PluginConfigRepository
	taskSvc    *TaskService

	// pushEnabled controls whether Roady writes status back to the external
	// tracker. On by default: a syncer that only reads is a mirror, not a
	// sync, and every plugin has always implemented Push.
	pushEnabled bool
}

// SetPushEnabled turns write-back to the external tracker on or off.
func (s *SyncService) SetPushEnabled(enabled bool) {
	s.pushEnabled = enabled
}

func NewSyncService(repo domain.WorkspaceRepository, taskSvc *TaskService) *SyncService {
	return &SyncService{repo: repo, taskSvc: taskSvc, pushEnabled: true}
}

// NewSyncServiceWithPlugins creates a SyncService with plugin config support
func NewSyncServiceWithPlugins(repo domain.WorkspaceRepository, pluginRepo PluginConfigRepository, taskSvc *TaskService) *SyncService {
	return &SyncService{repo: repo, pluginRepo: pluginRepo, taskSvc: taskSvc, pushEnabled: true}
}

// SyncWithNamedPlugin syncs using a named plugin configuration from plugins.yaml
func (s *SyncService) SyncWithNamedPlugin(name string) ([]string, error) {
	if s.pluginRepo == nil {
		return nil, fmt.Errorf("plugin configuration not available")
	}

	cfg, err := s.pluginRepo.GetPluginConfig(name)
	if err != nil {
		return nil, fmt.Errorf("failed to load plugin config '%s': %w", name, err)
	}

	return s.SyncWithPluginConfig(cfg.Binary, cfg.Config)
}

// SyncWithPlugin syncs using a plugin binary path (uses empty config, relies on env vars)
func (s *SyncService) SyncWithPlugin(pluginPath string) ([]string, error) {
	return s.SyncWithPluginConfig(pluginPath, map[string]string{})
}

// SyncWithPluginConfig syncs using a plugin binary path with explicit configuration
func (s *SyncService) SyncWithPluginConfig(pluginPath string, config map[string]string) ([]string, error) {
	loader := plugin.NewLoader()
	defer loader.Cleanup()

	syncer, err := loader.Load(pluginPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load plugin: %w", err)
	}

	plan, err := s.repo.LoadPlan()
	if err != nil {
		return nil, fmt.Errorf("failed to load plan: %w", err)
	}

	state, err := s.repo.LoadState()
	if err != nil {
		return nil, fmt.Errorf("failed to load state: %w", err)
	}

	if err := syncer.Init(config); err != nil {
		return nil, fmt.Errorf("failed to initialize plugin: %w", err)
	}

	result, err := syncer.Sync(plan, state)
	if err != nil {
		return nil, fmt.Errorf("failed to sync: %w", err)
	}

	results := []string{}
	provider := ProviderFromPluginPath(pluginPath)

	// 1. Handle Link Updates
	for id, ref := range result.LinkUpdates {
		if err := s.taskSvc.LinkTask(id, provider, ref); err != nil {
			results = append(results, fmt.Sprintf("Link Task %s: error (%v)", id, err))
		} else {
			results = append(results, fmt.Sprintf("Link Task %s: linked to %s (%s)", id, provider, ref.Identifier))
		}
	}

	// 2. Pull: apply the external tracker's statuses to Roady.
	results = append(results, s.applyInbound(state, result.StatusUpdates)...)

	// 3. Push: send Roady's status out for anything the tracker still
	// disagrees about. Inbound ran first, so a task the tracker moved now
	// matches and will not be pushed back — only tasks Roady moved remain,
	// which is what makes this converge without needing timestamps from
	// either side.
	if s.pushEnabled {
		results = append(results, s.applyOutbound(syncer, plan, result.StatusUpdates)...)
	}

	for _, e := range result.Errors {
		results = append(results, fmt.Sprintf("Plugin Error: %s", e))
	}

	return results, nil
}

// applyInbound moves Roady tasks to match the external tracker.
//
// Every status the tracker can report is honoured, not just done and
// in_progress: a task blocked in Jira must show as blocked in Roady, or the
// two records disagree and the plan-of-record claim is hollow. Multi-step
// transitions are walked one event at a time so each intermediate state is a
// real, audited transition rather than a status written past the FSM.
func (s *SyncService) applyInbound(state *planning.ExecutionState, updates map[string]planning.TaskStatus) []string {
	var results []string

	ids := make([]string, 0, len(updates))
	for id := range updates {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		target := updates[id]

		current := planning.StatusPending
		if state != nil {
			current = state.GetTaskStatus(id)
		}
		if current == target {
			continue
		}

		path, ok := planning.PathToStatus(current, target)
		if !ok {
			results = append(results, fmt.Sprintf("Status Task %s: skip (no path from %s to %s)", id, current, target))
			continue
		}

		failed := false
		for _, event := range path {
			if err := s.taskSvc.TransitionTask(id, event, "sync-plugin", ""); err != nil {
				results = append(results, fmt.Sprintf("Status Task %s: skip (%v)", id, err))
				failed = true
				break
			}
		}
		if !failed {
			results = append(results, fmt.Sprintf("Status Task %s: %s -> %s", id, current, target))
		}
	}

	return results
}

// applyOutbound pushes Roady's status to the external tracker for tasks whose
// remote status still differs.
//
// A push failure is reported but never aborts the sync: one unreachable issue
// must not stop the rest of the run, and the report tells the operator exactly
// which task did not land.
func (s *SyncService) applyOutbound(syncer domainPlugin.Syncer, plan *planning.Plan, external map[string]planning.TaskStatus) []string {
	var results []string

	// Re-read state so pushes reflect the inbound transitions just applied.
	state, err := s.repo.LoadState()
	if err != nil || state == nil {
		return []string{fmt.Sprintf("Push: skipped (cannot reload state: %v)", err)}
	}

	// Priority is plan data, not execution state, so it is read from the
	// plan rather than from state.json.
	priorities := map[string]planning.TaskPriority{}
	if plan != nil {
		for _, t := range plan.Tasks {
			priorities[t.ID] = t.Priority
		}
	}

	ids := make([]string, 0, len(external))
	for id := range external {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		local := state.GetTaskStatus(id)
		if local == external[id] {
			continue
		}

		if err := pushTask(syncer, id, local, priorities[id]); err != nil {
			results = append(results, fmt.Sprintf("Push Task %s: error (%v)", id, err))
			continue
		}
		results = append(results, fmt.Sprintf("Push Task %s: %s -> %s", id, external[id], local))
	}

	return results
}

// pushTask sends status, and attributes too when the plugin accepts them.
// A plugin that only implements Syncer still gets its status update, so
// attribute support is additive rather than a compatibility break.
func pushTask(syncer domainPlugin.Syncer, taskID string, status planning.TaskStatus, priority planning.TaskPriority) error {
	if fs, ok := syncer.(domainPlugin.FieldSyncer); ok {
		return fs.PushFields(taskID, domainPlugin.TaskFields{Status: status, Priority: priority})
	}
	return syncer.Push(taskID, status)
}

// ProviderFromPluginPath infers the provider name from a plugin binary path.
// The name is what task links are filed under, so an unrecognised plugin
// falling back to "external" would make every third-party syncer collide in
// ExternalRefs.
func ProviderFromPluginPath(pluginPath string) string {
	lower := strings.ToLower(pluginPath)
	for _, known := range []string{"github", "jira", "linear", "asana", "notion", "trello"} {
		if strings.Contains(lower, known) {
			return known
		}
	}

	// Fall back to the binary name with the conventional prefix stripped,
	// so a custom plugin still gets a stable identity of its own.
	base := filepath.Base(pluginPath)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	if trimmed := strings.TrimPrefix(base, "roady-plugin-"); trimmed != "" && trimmed != base {
		return trimmed
	}
	if base != "" {
		return base
	}
	return "external"
}

// ListPluginConfigs returns all configured plugin names
func (s *SyncService) ListPluginConfigs() ([]string, error) {
	if s.pluginRepo == nil {
		return nil, fmt.Errorf("plugin configuration not available")
	}

	configs, err := s.pluginRepo.LoadPluginConfigs()
	if err != nil {
		return nil, err
	}

	return configs.Names(), nil
}

// GetPluginConfig returns the configuration for a named plugin
func (s *SyncService) GetPluginConfig(name string) (*domainPlugin.PluginConfig, error) {
	if s.pluginRepo == nil {
		return nil, fmt.Errorf("plugin configuration not available")
	}

	return s.pluginRepo.GetPluginConfig(name)
}

// SetPluginConfig saves a plugin configuration
func (s *SyncService) SetPluginConfig(name string, cfg domainPlugin.PluginConfig) error {
	if s.pluginRepo == nil {
		return fmt.Errorf("plugin configuration not available")
	}

	return s.pluginRepo.SetPluginConfig(name, cfg)
}
