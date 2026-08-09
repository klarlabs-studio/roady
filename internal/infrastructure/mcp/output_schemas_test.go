package mcp

import (
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/billing"
	"github.com/felixgeelhaar/roady/pkg/domain/debt"
	"github.com/felixgeelhaar/roady/pkg/domain/drift"
	"github.com/felixgeelhaar/roady/pkg/domain/events"
	"github.com/felixgeelhaar/roady/pkg/domain/org"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	mcpschema "go.klarlabs.de/mcp/schema"
)

// TestOutputSchemasGenerate guards the OutputSchema failure mode: OutputSchema
// runs schema.Generate at tool registration, and if that errors the ToolBuilder
// short-circuits and the tool is silently never registered. Assert every
// advertised output type generates a schema, so a change to roady's domain
// model can't quietly drop a tool from the server.
func TestOutputSchemasGenerate(t *testing.T) {
	cases := []struct {
		name string
		v    any
	}{
		{"roady_plan_get", planning.Plan{}},
		{"roady_state_get", planning.ExecutionState{}},
		{"roady_spec_get", spec.ProductSpec{}},
		{"roady_snapshot_get", snapshotResp{}},
		{"roady_forecast", forecastResp{}},
		{"roady_usage_get", domain.UsageStats{}},
		{"roady_drift_detect", drift.Report{}},
		{"roady_org_status", org.OrgMetrics{}},
		{"roady_deps_scan", application.ScanResult{}},
		{"roady_debt_report", debt.DebtReport{}},
		{"roady_debt_summary", application.DebtSummary{}},
		{"roady_debt_trend", events.DriftTrend{}},
		{"roady_plan_prioritize", planning.PrioritySuggestions{}},
		{"roady_spec_review", spec.SpecReview{}},
		{"roady_cost_report", billing.CostReport{}},
		{"roady_cost_budget", billing.BudgetStatus{}},
	}
	for _, c := range cases {
		if _, err := mcpschema.Generate(c.v); err != nil {
			t.Errorf("OutputSchema(%s): schema.Generate failed — the tool would silently not register: %v", c.name, err)
		}
	}
}
