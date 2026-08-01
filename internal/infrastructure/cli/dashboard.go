package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Interactive TUI dashboard",
	Long: `Open an interactive terminal dashboard showing project status, tasks,
and drift at a glance.

For a shareable progress document — one a lead or stakeholder can read
without installing anything — use 'roady report' instead. To push a
summary to a chat channel on a schedule, use 'roady notify digest'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Getenv("ROADY_SKIP_DASHBOARD_RUN") == "true" {
			return nil
		}
		p := tea.NewProgram(initialModel())
		if _, err := p.Run(); err != nil {
			return fmt.Errorf("dashboard run failed: %w", err)
		}
		return nil
	},
}

func init() {
	RootCmd.AddCommand(dashboardCmd)
}

// Styles
var baseStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("240"))

var headerStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("#FAFAFA")).
	Background(lipgloss.Color("#7D56F4")).
	PaddingLeft(1).
	PaddingRight(1)

var statusDone = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
var statusErr = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

type model struct {
	table   table.Model
	project string
	version string
	usage   int
	limit   int
	drift   []string
	err     error
}

func initialModel() model {
	services, err := loadServicesForCurrentDir()
	if err != nil {
		return model{err: err}
	}
	repo := services.Workspace.Repo

	// Load Data
	spec, err := repo.LoadSpec()
	if err != nil {
		return model{err: err}
	}

	plan, err := repo.LoadPlan()
	if err != nil {
		return model{err: err}
	}

	state, err := repo.LoadState()
	if err != nil {
		return model{err: err}
	}

	stats, _ := repo.LoadUsage()
	usageCount := 0
	if stats != nil {
		for _, c := range stats.ProviderStats {
			usageCount += c
		}
	}

	cfg, _ := repo.LoadPolicy()
	tokenLimit := 0
	if cfg != nil {
		tokenLimit = cfg.TokenLimit
	}

	// Setup Table
	columns := []table.Column{
		{Title: "Status", Width: 10},
		{Title: "Owner", Width: 15},
		{Title: "Priority", Width: 8},
		{Title: "Task", Width: 40},
		{Title: "ID", Width: 20},
	}

	rows := []table.Row{}
	if plan != nil {
		for _, t := range plan.Tasks {
			status := "pending"
			owner := "-"
			if res, ok := state.TaskStates[t.ID]; ok {
				status = string(res.Status)
				if res.Owner != "" {
					owner = res.Owner
				}
			}
			rows = append(rows, table.Row{status, owner, string(t.Priority), t.Title, t.ID})
		}
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240"))

	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229"))

	t.SetStyles(s)

	// Drift Detection (Quick check)
	report, _ := services.Drift.DetectDrift(context.Background())
	driftMsgs := []string{}
	if report != nil {
		for _, issue := range report.Issues {
			driftMsgs = append(driftMsgs, fmt.Sprintf("[%s] %s", issue.Severity, issue.Message))
		}
	}

	return model{
		table:   t,
		project: spec.Title,
		version: spec.Version,
		usage:   usageCount,
		limit:   tokenLimit,
		drift:   driftMsgs,
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error loading dashboard: %v\nPress q to quit.", m.err)
	}

	header := headerStyle.Render(fmt.Sprintf("%s v%s", m.project, m.version))

	budgetText := fmt.Sprintf("AI Budget: %d", m.usage)
	if m.limit > 0 {
		budgetText = fmt.Sprintf("AI Budget: %d / %d", m.usage, m.limit)
	}

	driftView := ""
	if len(m.drift) > 0 {
		driftView = statusErr.Render("\nDRIFT DETECTED:\n")
		for _, d := range m.drift {
			driftView += fmt.Sprintf("- %s\n", d)
		}
	} else {
		driftView = statusDone.Render("\nSystem Sync: OK")
	}

	return baseStyle.Render(

		lipgloss.JoinVertical(lipgloss.Left,

			header,

			budgetText,

			"\nTask Plan:",

			m.table.View(),

			driftView,

			"\n[q] Quit  [Up/Down] Navigate",
		),
	) + "\n"

}
