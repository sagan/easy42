package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CronTask represents a scheduled Looking Glass task executed periodically by node agents
type CronTask struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	TaskType    string            `json:"task_type"` // "ping" or "tcp_ping"
	CronExpr    string            `json:"cron_expr"` // e.g. "0 */1 * * *" or "*/5 * * * *"
	Nodes       []string          `json:"nodes"`     // Node names assigned to execute this task
	Params      map[string]string `json:"params"`    // Task parameters (e.g. target, count)
	TimeoutSec  int               `json:"timeout_sec,omitempty"`
	Enabled     bool              `json:"enabled"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// CronTaskResultRecord represents an executed cron task result stored in SQLite
type CronTaskResultRecord struct {
	ID              int64   `json:"id"`
	TaskID          string  `json:"task_id"`
	TaskName        string  `json:"task_name"`
	TaskType        string  `json:"task_type"`
	NodeName        string  `json:"node_name"`
	ExecutedAt      int64   `json:"executed_at"` // Unix timestamp in seconds
	DurationMs      int64   `json:"duration_ms"`
	ExitCode        int     `json:"exit_code"`
	Error           string  `json:"error,omitempty"`
	Target          string  `json:"target"`
	PacketsSent     int     `json:"packets_sent"`
	PacketsReceived int     `json:"packets_received"`
	PacketLossPct   float64 `json:"packet_loss_pct"`
	MinRTTMs        float64 `json:"min_rtt_ms"`
	AvgRTTMs        float64 `json:"avg_rtt_ms"`
	MaxRTTMs        float64 `json:"max_rtt_ms"`
	MdevRTTMs       float64 `json:"mdev_rtt_ms"`
	RawOutput       string  `json:"raw_output"`
	ParsedJSON      string  `json:"parsed_json,omitempty"`
}

// CronTaskStatsSummary aggregates execution statistics for a task on a node
type CronTaskStatsSummary struct {
	TaskID         string  `json:"task_id"`
	TaskName       string  `json:"task_name"`
	TaskType       string  `json:"task_type"`
	Target         string  `json:"target"`
	TotalRuns      int     `json:"total_runs"`
	LatestRunTime  int64   `json:"latest_run_time"`
	LatestAvgRTT   float64 `json:"latest_avg_rtt_ms"`
	LatestLossPct  float64 `json:"latest_loss_pct"`
	OverallAvgRTT  float64 `json:"overall_avg_rtt_ms"`
	OverallLossPct float64 `json:"overall_loss_pct"`
	LastStatus     string  `json:"last_status"` // "ok", "degraded", "failed"
}

// NodeCronTasksResultsResponse wraps results for a node details page
type NodeCronTasksResultsResponse struct {
	Results []CronTaskResultRecord `json:"results"`
	Summary []CronTaskStatsSummary `json:"summary"`
	Tasks   []CronTask             `json:"tasks"`
}

// ValidateCronExpr checks if a cron expression has a valid syntax (5 or 6 fields)
func ValidateCronExpr(expr string) error {
	fields := strings.Fields(strings.TrimSpace(expr))
	if len(fields) != 5 && len(fields) != 6 {
		return fmt.Errorf("cron expression must have 5 fields (min hour dom mon dow) or 6 fields (sec min hour dom mon dow), got %d", len(fields))
	}

	fieldLimits := []struct {
		name string
		min  int
		max  int
	}{
		{"minute", 0, 59},
		{"hour", 0, 23},
		{"day of month", 1, 31},
		{"month", 1, 12},
		{"day of week", 0, 7},
	}

	if len(fields) == 6 {
		fieldLimits = append([]struct {
			name string
			min  int
			max  int
		}{{"second", 0, 59}}, fieldLimits...)
	}

	for i, f := range fields {
		limit := fieldLimits[i]
		if err := validateCronField(f, limit.min, limit.max, limit.name); err != nil {
			return err
		}
	}
	return nil
}

func validateCronField(field string, min, max int, name string) error {
	if field == "*" || field == "?" {
		return nil
	}

	parts := strings.Split(field, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return fmt.Errorf("empty item in %s field", name)
		}

		if strings.Contains(part, "/") {
			subParts := strings.Split(part, "/")
			if len(subParts) != 2 {
				return fmt.Errorf("invalid step syntax in %s field: %s", name, part)
			}
			base := subParts[0]
			stepStr := subParts[1]
			step, err := strconv.Atoi(stepStr)
			if err != nil || step <= 0 {
				return fmt.Errorf("invalid step value in %s field: %s", name, stepStr)
			}

			if base != "*" && base != "" {
				if err := validateRangeOrNum(base, min, max, name); err != nil {
					return err
				}
			}
			continue
		}

		if err := validateRangeOrNum(part, min, max, name); err != nil {
			return err
		}
	}
	return nil
}

func validateRangeOrNum(part string, min, max int, name string) error {
	if strings.Contains(part, "-") {
		rangeParts := strings.Split(part, "-")
		if len(rangeParts) != 2 {
			return fmt.Errorf("invalid range syntax in %s field: %s", name, part)
		}
		start, err1 := strconv.Atoi(rangeParts[0])
		end, err2 := strconv.Atoi(rangeParts[1])
		if err1 != nil || err2 != nil || start < min || end > max || start > end {
			return fmt.Errorf("range out of bounds [%d-%d] in %s field: %s", min, max, name, part)
		}
		return nil
	}

	val, err := strconv.Atoi(part)
	if err != nil || val < min || val > max {
		return fmt.Errorf("value out of bounds [%d-%d] in %s field: %s", min, max, name, part)
	}
	return nil
}
