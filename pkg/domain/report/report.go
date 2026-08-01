// Package report models a point-in-time progress report intended for people
// who do not run the CLI — leads, stakeholders, anyone who needs to know where
// a project stands without opening it.
//
// The model is deliberately render-agnostic: it carries no formatting, so the
// same report can be emitted as Markdown, HTML, or JSON.
package report

import "time"

// Report is a complete progress snapshot for one project.
type Report struct {
	Project     string       `json:"project"`
	GeneratedAt time.Time    `json:"generated_at"`
	Since       *time.Time   `json:"since,omitempty"`
	Progress    Progress     `json:"progress"`
	Forecast    *Forecast    `json:"forecast,omitempty"`
	Assignments []Assignment `json:"assignments"`
	Risks       []Risk       `json:"risks"`
	Changes     []Change     `json:"changes"`
}

// Progress counts tasks by status. Percent is the share of tasks that have
// reached done or verified.
type Progress struct {
	Total      int     `json:"total"`
	Done       int     `json:"done"`
	Verified   int     `json:"verified"`
	InProgress int     `json:"in_progress"`
	Blocked    int     `json:"blocked"`
	Ready      int     `json:"ready"`
	Pending    int     `json:"pending"`
	Percent    float64 `json:"percent"`
}

// Forecast projects completion from observed velocity. Days are calendar days
// from GeneratedAt. LowDays and HighDays bound the confidence interval.
type Forecast struct {
	Velocity      float64 `json:"velocity"`
	EstimatedDays float64 `json:"estimated_days"`
	LowDays       float64 `json:"low_days"`
	HighDays      float64 `json:"high_days"`
	Trend         string  `json:"trend"`
	DataPoints    int     `json:"data_points"`
}

// Reliable reports whether the forecast rests on enough observations to be
// worth showing. Below this bar a velocity number invites false confidence,
// so renderers should say "not enough data" instead of printing a figure.
func (f *Forecast) Reliable() bool {
	return f != nil && f.DataPoints >= minForecastDataPoints && f.Velocity > 0
}

const minForecastDataPoints = 3

// Assignment is one person's or agent's slice of the work.
type Assignment struct {
	Owner      string     `json:"owner"`
	Active     int        `json:"active"`
	Blocked    int        `json:"blocked"`
	Done       int        `json:"done"`
	OpenTasks  []TaskLine `json:"open_tasks"`
	Unassigned bool       `json:"unassigned"`
}

// TaskLine is the minimum needed to identify a task in a report.
type TaskLine struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// Risk is anything a stakeholder should know is going wrong: drift between
// intent and reality, or debt that has sat unresolved.
type Risk struct {
	Severity    string `json:"severity"`
	Kind        string `json:"kind"`
	Component   string `json:"component"`
	Message     string `json:"message"`
	DaysPending int    `json:"days_pending,omitempty"`
}

// Change is a notable event in the reporting window.
type Change struct {
	At      time.Time `json:"at"`
	Action  string    `json:"action"`
	Actor   string    `json:"actor"`
	Summary string    `json:"summary"`
}

// HasRisks reports whether anything needs escalating.
func (r *Report) HasRisks() bool { return len(r.Risks) > 0 }

// Headline gives a one-line status suitable for a Slack message or an email
// subject. It leads with risk when there is any, because that is the part a
// stakeholder must not miss.
func (r *Report) Headline() string {
	switch {
	case r.Progress.Total == 0:
		return "No plan yet"
	case len(r.Risks) > 0:
		return pct(r.Progress.Percent) + " complete, " + plural(len(r.Risks), "risk", "risks") + " open"
	default:
		return pct(r.Progress.Percent) + " complete, no open risks"
	}
}
