package plugin

import (
	"net/rpc"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	goplugin "github.com/hashicorp/go-plugin"
)

// Syncer is the interface that plugins must implement.
type Syncer interface {
	// Init ensures the plugin can connect (auth check)
	Init(config map[string]string) error

	// Sync performs the bi-directional synchronization
	Sync(plan *planning.Plan, state *planning.ExecutionState) (*SyncResult, error)

	// Push sends a status update for a specific task to the external system
	Push(taskID string, status planning.TaskStatus) error
}

// TaskFields carries the task attributes Roady pushes outward alongside
// status.
//
// Priority only travels outward. It originates in the spec
// (Requirement.Priority), flows into the plan, and is rebuilt from the spec
// every time a plan is generated — PlanReconciler replaces each task with the
// proposed structure — so a priority written inward from a tracker would be
// silently discarded on the next `roady plan generate`. Rather than offer a
// field that quietly loses data, the direction is one-way by design.
//
// Estimate and assignee are deliberately absent. Roady's estimate is a free
// string ("4h") and its owner is a free string, while trackers use story
// points and user IDs; mapping either without a per-provider identity
// resolution would overwrite real data with a guess.
type TaskFields struct {
	Status   planning.TaskStatus
	Priority planning.TaskPriority
}

// FieldSyncer is an optional extension of Syncer. A plugin that implements it
// receives task attributes as well as status; one that does not keeps working
// unchanged through Push, so adding this broke no existing plugin.
//
// Not every tracker has somewhere sensible to put every field — Trello has no
// native priority — so implementing this is a per-provider choice.
type FieldSyncer interface {
	Syncer

	// PushFields sends status and attributes together. Implementations must
	// treat a zero-valued field as "not specified" and leave it alone.
	PushFields(taskID string, fields TaskFields) error
}

// SyncResult captures the outcome of a sync operation
type SyncResult struct {
	StatusUpdates map[string]planning.TaskStatus  `json:"status_updates"`
	LinkUpdates   map[string]planning.ExternalRef `json:"link_updates"`
	Errors        []string                        `json:"errors"`
}

// SyncerPlugin is the implementation of plugin.Plugin so we can serve/consume this.
type SyncerPlugin struct {
	Impl Syncer
}

func (p *SyncerPlugin) Server(*goplugin.MuxBroker) (interface{}, error) {
	return &SyncerRPCServer{Impl: p.Impl}, nil
}

func (p *SyncerPlugin) Client(b *goplugin.MuxBroker, c *rpc.Client) (interface{}, error) {
	return &SyncerRPCClient{Client: c}, nil
}

// RPC Client/Server wrappers
type SyncArgs struct {
	Plan  *planning.Plan
	State *planning.ExecutionState
}

type PushArgs struct {
	TaskID string
	Status planning.TaskStatus
	// Priority is additive: net/rpc ignores fields a peer does not know, so
	// an older plugin binary simply never sees it.
	Priority planning.TaskPriority
}

type SyncerRPCClient struct{ Client *rpc.Client }

func (g *SyncerRPCClient) Init(config map[string]string) error {
	var resp interface{}
	return g.Client.Call("Plugin.Init", config, &resp)
}

func (g *SyncerRPCClient) Sync(plan *planning.Plan, state *planning.ExecutionState) (*SyncResult, error) {
	var resp SyncResult
	args := &SyncArgs{Plan: plan, State: state}
	err := g.Client.Call("Plugin.Sync", args, &resp)
	return &resp, err
}

func (g *SyncerRPCClient) Push(taskID string, status planning.TaskStatus) error {
	var resp interface{}
	args := &PushArgs{TaskID: taskID, Status: status}
	return g.Client.Call("Plugin.Push", args, &resp)
}

// PushFields lets the host send attributes without knowing whether the plugin
// on the other end supports them; the server side decides.
func (g *SyncerRPCClient) PushFields(taskID string, fields TaskFields) error {
	var resp interface{}
	args := &PushArgs{TaskID: taskID, Status: fields.Status, Priority: fields.Priority}
	return g.Client.Call("Plugin.Push", args, &resp)
}

type SyncerRPCServer struct{ Impl Syncer }

func (s *SyncerRPCServer) Init(config map[string]string, resp *interface{}) error {
	return s.Impl.Init(config)
}

func (s *SyncerRPCServer) Sync(args *SyncArgs, resp *SyncResult) error {
	result, err := s.Impl.Sync(args.Plan, args.State)
	if result != nil {
		*resp = *result
	}
	return err
}

func (s *SyncerRPCServer) Push(args *PushArgs, resp *interface{}) error {
	// Prefer the richer call when this plugin implements it, so a host that
	// sends attributes reaches a plugin that can use them without either
	// side negotiating a version.
	if fs, ok := s.Impl.(FieldSyncer); ok {
		return fs.PushFields(args.TaskID, TaskFields{
			Status:   args.Status,
			Priority: args.Priority,
		})
	}
	return s.Impl.Push(args.TaskID, args.Status)
}
