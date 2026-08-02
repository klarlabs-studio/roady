package cli

import (
	"encoding/json"
	"fmt"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/spf13/cobra"
)

var orgJSON bool

var orgCmd = &cobra.Command{
	Use:   "org",
	Short: "Organizational views across multiple projects",
}

var orgStatusCmd = &cobra.Command{
	Use:   "status [root-dir]",
	Short: "Show status overview of all Roady projects in a directory",
	RunE: func(cmd *cobra.Command, args []string) error {
		root := "."
		if len(args) > 0 {
			root = args[0]
		}

		svc := application.NewOrgService(root)
		metrics, err := svc.AggregateMetrics()
		if err != nil {
			return err
		}

		if len(metrics.Projects) == 0 {
			fmt.Println("No Roady projects found.")
			printMemberWarnings(metrics.Warnings)
			return nil
		}

		if orgJSON {
			data, err := json.MarshalIndent(metrics, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}

		columns := []table.Column{
			{Title: "Project", Width: 30},
			{Title: "Progress", Width: 10},
			{Title: "Vrf", Width: 5},
			{Title: "WIP", Width: 5},
			{Title: "Total", Width: 5},
			{Title: "Path", Width: 40},
		}

		rows := []table.Row{}
		for _, pm := range metrics.Projects {
			progress := "0%"
			if pm.Total > 0 {
				progress = fmt.Sprintf("%.1f%%", pm.Progress)
			}
			rows = append(rows, table.Row{
				pm.Name,
				progress,
				fmt.Sprintf("%d", pm.Verified),
				fmt.Sprintf("%d", pm.WIP),
				fmt.Sprintf("%d", pm.Total),
				pm.Path,
			})
		}

		// Summary row
		avgProgress := "0%"
		if metrics.TotalProjects > 0 {
			avgProgress = fmt.Sprintf("%.1f%%", metrics.AvgProgress)
		}
		rows = append(rows, table.Row{
			fmt.Sprintf("TOTAL (%d)", metrics.TotalProjects),
			avgProgress,
			fmt.Sprintf("%d", metrics.TotalVerified),
			fmt.Sprintf("%d", metrics.TotalWIP),
			fmt.Sprintf("%d", metrics.TotalTasks),
			"",
		})

		t := table.New(
			table.WithColumns(columns),
			table.WithRows(rows),
			table.WithHeight(len(rows)+1),
		)

		s := table.DefaultStyles()
		s.Header = s.Header.
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("240")).
			Bold(true)
		s.Selected = lipgloss.NewStyle()
		t.SetStyles(s)

		fmt.Printf("Organizational Status (%d projects)\n", metrics.TotalProjects)
		fmt.Println(t.View())
		printMemberWarnings(metrics.Warnings)
		return nil
	},
}

var orgPolicyCmd = &cobra.Command{
	Use:   "policy [project-path]",
	Short: "Show merged policy (org defaults + project overrides)",
	RunE: func(cmd *cobra.Command, args []string) error {
		root := "."
		if len(args) > 0 {
			root = args[0]
		}

		// Use parent of project as org root to find org.yaml
		svc := application.NewOrgService(root)
		projectPath := root
		if len(args) > 0 {
			projectPath = args[0]
		}

		merged, err := svc.LoadMergedPolicy(projectPath)
		if err != nil {
			return err
		}

		if orgJSON {
			data, err := json.MarshalIndent(merged, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}

		fmt.Printf("Merged Policy:\n")
		fmt.Printf("  Max WIP:     %d\n", merged.MaxWIP)
		fmt.Printf("  Allow AI:    %v\n", merged.AllowAI)
		fmt.Printf("  Token Limit: %d\n", merged.TokenLimit)
		return nil
	},
}

var orgDriftCmd = &cobra.Command{
	Use:   "drift [root-dir]",
	Short: "Detect drift across all projects in the directory tree",
	RunE: func(cmd *cobra.Command, args []string) error {
		root := "."
		if len(args) > 0 {
			root = args[0]
		}

		svc := application.NewOrgService(root)
		report, err := svc.DetectCrossDrift()
		if err != nil {
			return err
		}

		if len(report.Projects) == 0 {
			fmt.Println("No Roady projects found.")
			printMemberWarnings(report.Warnings)
			return nil
		}

		if orgJSON {
			data, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}

		fmt.Printf("Cross-Project Drift Report (%d projects, %d total issues)\n\n", len(report.Projects), report.TotalIssues)
		for _, p := range report.Projects {
			status := "clean"
			if p.HasDrift {
				status = fmt.Sprintf("%d issues", p.IssueCount)
			}
			fmt.Printf("  %-30s %s\n", p.Name, status)
		}
		printMemberWarnings(report.Warnings)
		return nil
	},
}

func init() {
	orgStatusCmd.Flags().BoolVar(&orgJSON, "json", false, "Output as JSON")
	orgPolicyCmd.Flags().BoolVar(&orgJSON, "json", false, "Output as JSON")
	orgDriftCmd.Flags().BoolVar(&orgJSON, "json", false, "Output as JSON")
	orgCmd.AddCommand(orgStatusCmd)
	orgCmd.AddCommand(orgPolicyCmd)
	orgCmd.AddCommand(orgDriftCmd)
	RootCmd.AddCommand(orgCmd)
}
