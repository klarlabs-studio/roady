package report

import "testing"

func TestReportHeadline(t *testing.T) {
	tests := []struct {
		name   string
		report Report
		want   string
	}{
		{
			name:   "no plan",
			report: Report{},
			want:   "No plan yet",
		},
		{
			name:   "clean run",
			report: Report{Progress: Progress{Total: 4, Percent: 50}},
			want:   "50% complete, no open risks",
		},
		{
			name: "single risk uses singular",
			report: Report{
				Progress: Progress{Total: 4, Percent: 25},
				Risks:    []Risk{{Severity: "high"}},
			},
			want: "25% complete, 1 risk open",
		},
		{
			name: "several risks",
			report: Report{
				Progress: Progress{Total: 8, Percent: 12.5},
				Risks:    []Risk{{Severity: "high"}, {Severity: "low"}},
			},
			want: "12.5% complete, 2 risks open",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.report.Headline(); got != tt.want {
				t.Errorf("Headline() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestForecastReliable(t *testing.T) {
	tests := []struct {
		name     string
		forecast *Forecast
		want     bool
	}{
		{name: "nil forecast", forecast: nil, want: false},
		{name: "too few data points", forecast: &Forecast{Velocity: 2, DataPoints: 2}, want: false},
		{name: "zero velocity", forecast: &Forecast{Velocity: 0, DataPoints: 10}, want: false},
		{name: "enough data", forecast: &Forecast{Velocity: 1.5, DataPoints: 3}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.forecast.Reliable(); got != tt.want {
				t.Errorf("Reliable() = %v, want %v", got, tt.want)
			}
		})
	}
}
