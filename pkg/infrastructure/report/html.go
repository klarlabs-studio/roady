package report

import (
	"bytes"
	"html/template"
	"strconv"

	domainreport "github.com/felixgeelhaar/roady/pkg/domain/report"
)

// HTML renders a report as a single self-contained page: no scripts, no
// external assets, no fonts to fetch. It is meant to be emailed, committed,
// or published to a static host and read once — not operated.
func HTML(r *domainreport.Report) (string, error) {
	var buf bytes.Buffer
	if err := htmlTemplate.Execute(&buf, newHTMLView(r)); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// htmlView is the flattened, presentation-ready shape the template consumes,
// so the template itself stays free of logic.
type htmlView struct {
	Title       string
	Headline    string
	GeneratedAt string
	Since       string
	Progress    domainreport.Progress
	PercentText string
	StatusRows  []statusRow
	Forecast    *forecastView
	Risks       []domainreport.Risk
	Assignments []assignmentView
	Changes     []changeView
}

type statusRow struct {
	Label string
	Count int
	// Share is the percentage width of this row's bar, 0-100.
	Share float64
}

type forecastView struct {
	Reliable bool
	Velocity string
	Estimate string
	Range    string
	Trend    string
}

type assignmentView struct {
	Name      string
	Active    int
	Blocked   int
	Done      int
	OpenTasks []domainreport.TaskLine
}

type changeView struct {
	Date    string
	Summary string
	Actor   string
}

func newHTMLView(r *domainreport.Report) htmlView {
	title := r.Project
	if title == "" {
		title = "Project"
	}

	v := htmlView{
		Title:       title,
		Headline:    r.Headline(),
		GeneratedAt: r.GeneratedAt.Format("2006-01-02 15:04 MST"),
		Progress:    r.Progress,
		PercentText: formatPercent(r.Progress.Percent),
		Risks:       r.Risks,
	}

	if r.Since != nil {
		v.Since = r.Since.Format("2006-01-02")
	}

	total := r.Progress.Total
	for _, row := range []struct {
		label string
		count int
	}{
		{"Verified", r.Progress.Verified},
		{"Done", r.Progress.Done},
		{"In progress", r.Progress.InProgress},
		{"Blocked", r.Progress.Blocked},
		{"Ready", r.Progress.Ready},
		{"Pending", r.Progress.Pending},
	} {
		if row.count == 0 {
			continue
		}
		share := 0.0
		if total > 0 {
			share = float64(row.count) / float64(total) * 100
		}
		v.StatusRows = append(v.StatusRows, statusRow{Label: row.label, Count: row.count, Share: share})
	}

	if r.Forecast != nil {
		fv := &forecastView{Reliable: r.Forecast.Reliable(), Trend: r.Forecast.Trend}
		if fv.Reliable {
			fv.Velocity = formatVelocity(r.Forecast.Velocity)
			fv.Estimate = formatDays(r.Forecast.EstimatedDays)
			if r.Forecast.HighDays > 0 {
				fv.Range = formatDays(r.Forecast.LowDays) + " to " + formatDays(r.Forecast.HighDays)
			}
		}
		v.Forecast = fv
	}

	for _, a := range r.Assignments {
		name := a.Owner
		if a.Unassigned {
			name = "Unassigned"
		}
		v.Assignments = append(v.Assignments, assignmentView{
			Name:      name,
			Active:    a.Active,
			Blocked:   a.Blocked,
			Done:      a.Done,
			OpenTasks: a.OpenTasks,
		})
	}

	for _, c := range r.Changes {
		v.Changes = append(v.Changes, changeView{
			Date:    c.At.Format("2006-01-02"),
			Summary: c.Summary,
			Actor:   c.Actor,
		})
	}

	return v
}

func formatVelocity(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64) + " tasks/day"
}

var htmlTemplate = template.Must(template.New("report").Funcs(template.FuncMap{
	"severityClass": severityClass,
}).Parse(htmlSource))

// severityClass maps a severity onto a CSS class so styling stays in the
// stylesheet rather than in the data.
func severityClass(severity string) string {
	switch severity {
	case "critical", "high":
		return "sev-high"
	case "medium":
		return "sev-medium"
	default:
		return "sev-low"
	}
}

const htmlSource = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} — progress report</title>
<style>
  :root {
    color-scheme: light dark;
    --bg: #ffffff; --fg: #1a1a1a; --muted: #666; --line: #e2e2e2;
    --bar: #3b6ea5; --bar-track: #eceff3;
    --high: #b3261e; --medium: #9a6700; --low: #57606a;
  }
  @media (prefers-color-scheme: dark) {
    :root {
      --bg: #16181c; --fg: #e6e6e6; --muted: #9aa0a6; --line: #2c2f36;
      --bar: #6aa3e0; --bar-track: #23262c;
      --high: #ff7b72; --medium: #d29922; --low: #8b949e;
    }
  }
  * { box-sizing: border-box; }
  body {
    margin: 0 auto; padding: 2.5rem 1.25rem; max-width: 46rem;
    background: var(--bg); color: var(--fg);
    font: 16px/1.6 -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  }
  h1 { font-size: 1.6rem; margin: 0 0 .25rem; }
  h2 { font-size: 1.05rem; text-transform: uppercase; letter-spacing: .06em;
       color: var(--muted); margin: 2.5rem 0 .75rem; font-weight: 600; }
  .headline { font-size: 1.15rem; font-weight: 600; margin: 0 0 .25rem; }
  .meta { color: var(--muted); font-size: .875rem; margin: 0 0 1rem; }
  table { border-collapse: collapse; width: 100%; font-size: .9rem; }
  th, td { text-align: left; padding: .45rem .6rem; border-bottom: 1px solid var(--line); vertical-align: top; }
  th { color: var(--muted); font-weight: 600; }
  .bar-row td { border: 0; padding: .2rem .6rem .2rem 0; }
  .bar-label { width: 8rem; color: var(--muted); font-size: .875rem; }
  .bar-track { background: var(--bar-track); border-radius: 3px; height: .55rem; width: 100%; }
  .bar-fill { background: var(--bar); border-radius: 3px; height: 100%; }
  .bar-count { width: 3rem; text-align: right; font-variant-numeric: tabular-nums; }
  .sev-high { color: var(--high); font-weight: 600; }
  .sev-medium { color: var(--medium); font-weight: 600; }
  .sev-low { color: var(--low); }
  .owner { margin: 1.25rem 0 .35rem; font-weight: 600; }
  .owner-counts { color: var(--muted); font-weight: 400; font-size: .875rem; }
  ul { margin: .35rem 0; padding-left: 1.1rem; }
  li { margin: .15rem 0; }
  code { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: .85em;
         background: var(--bar-track); padding: .1em .35em; border-radius: 3px; }
  .empty { color: var(--muted); font-style: italic; }
  .changes li { color: var(--fg); }
  .changes .when { color: var(--muted); font-variant-numeric: tabular-nums; }
  @media print { body { max-width: none; padding: 0; } h2 { page-break-after: avoid; } }
</style>
</head>
<body>
<h1>{{.Title}}</h1>
<p class="headline">{{.Headline}}</p>
<p class="meta">Generated {{.GeneratedAt}}{{if .Since}} · covering changes since {{.Since}}{{end}}</p>

<h2>Progress</h2>
{{if eq .Progress.Total 0}}
  <p class="empty">No plan has been generated yet.</p>
{{else}}
  <p>{{.PercentText}} complete — {{.Progress.Total}} tasks total.</p>
  <table>
    {{range .StatusRows}}
    <tr class="bar-row">
      <td class="bar-label">{{.Label}}</td>
      <td><div class="bar-track"><div class="bar-fill" style="width:{{printf "%.1f" .Share}}%"></div></div></td>
      <td class="bar-count">{{.Count}}</td>
    </tr>
    {{end}}
  </table>
{{end}}

{{with .Forecast}}
<h2>Forecast</h2>
{{if .Reliable}}
  <p>At {{.Velocity}}, completion is expected in about <strong>{{.Estimate}}</strong>{{if .Range}} (range {{.Range}}){{end}}.</p>
  {{if .Trend}}<p class="meta">Velocity trend: {{.Trend}}.</p>{{end}}
{{else}}
  <p class="empty">Not enough completed work yet to forecast a completion date.</p>
{{end}}
{{end}}

<h2>Risks</h2>
{{if .Risks}}
<table>
  <tr><th>Severity</th><th>Kind</th><th>Component</th><th>Detail</th></tr>
  {{range .Risks}}
  <tr>
    <td class="{{severityClass .Severity}}">{{.Severity}}</td>
    <td>{{.Kind}}</td>
    <td>{{if .Component}}{{.Component}}{{else}}—{{end}}</td>
    <td>{{.Message}}{{if .DaysPending}} <span class="meta">(open {{.DaysPending}} days)</span>{{end}}</td>
  </tr>
  {{end}}
</table>
{{else}}
<p class="empty">None open.</p>
{{end}}

<h2>Who is on what</h2>
{{if .Assignments}}
  {{range .Assignments}}
  <p class="owner">{{.Name}}
    <span class="owner-counts">— {{.Active}} active, {{.Blocked}} blocked, {{.Done}} done</span></p>
  {{if .OpenTasks}}
  <ul>
    {{range .OpenTasks}}<li><code>{{.ID}}</code> {{.Title}} <span class="meta">({{.Status}})</span></li>{{end}}
  </ul>
  {{else}}
  <p class="empty">No open tasks.</p>
  {{end}}
  {{end}}
{{else}}
<p class="empty">No tasks yet.</p>
{{end}}

<h2>What changed</h2>
{{if .Changes}}
<ul class="changes">
  {{range .Changes}}<li><span class="when">{{.Date}}</span> — {{.Summary}}{{if .Actor}} <span class="meta">({{.Actor}})</span>{{end}}</li>{{end}}
</ul>
{{else}}
<p class="empty">No recorded activity in this window.</p>
{{end}}
</body>
</html>
`
