package config

import (
	"database/sql"
	"encoding/json"
	"errors"
)

// PingTableItem represents the ping result or status for a pair of nodes
type PingTableItem struct {
	FromNode      string   `json:"from_node"`
	ToNode        string   `json:"to_node"`
	TargetIP      string   `json:"target_ip,omitempty"`
	Interface     string   `json:"interface,omitempty"`
	LatencyMs     *float64 `json:"latency_ms,omitempty"`
	PacketLossPct float64  `json:"packet_loss_pct"`
	Status        string   `json:"status"` // "pending", "running", "success", "failed", "unreachable", "no_agent", "no_link"
	Error         string   `json:"error,omitempty"`
}

// PingTableTask represents a recorded ping table measurement task
type PingTableTask struct {
	ID             string                               `json:"id"`
	CreatedAt      int64                                `json:"created_at"`
	FinishedAt     int64                                `json:"finished_at"`
	LatencyType    string                               `json:"latency_type"` // "e2e" or "link"
	Status         string                               `json:"status"`       // "running", "completed", "cancelled", "failed"
	TotalPairs     int                                  `json:"total_pairs"`
	CompletedPairs int                                  `json:"completed_pairs"`
	Nodes          []string                             `json:"nodes"`
	Results        map[string]map[string]*PingTableItem `json:"results"` // [from][to] -> item
	Error          string                               `json:"error,omitempty"`
}

// PingTableTaskSummary provides metadata for history listing
type PingTableTaskSummary struct {
	ID             string `json:"id"`
	CreatedAt      int64  `json:"created_at"`
	FinishedAt     int64  `json:"finished_at"`
	LatencyType    string `json:"latency_type"`
	Status         string `json:"status"`
	TotalPairs     int    `json:"total_pairs"`
	CompletedPairs int    `json:"completed_pairs"`
	NodeCount      int    `json:"node_count"`
	Error          string `json:"error,omitempty"`
}

// RecordPingTableTask records or updates execution state of a ping table task
func (s *StateStore) RecordPingTableTask(task PingTableTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.initDBLocked(); err != nil {
		return err
	}

	nodesJSON, err := json.Marshal(task.Nodes)
	if err != nil {
		nodesJSON = []byte("[]")
	}

	resultsJSON, err := json.Marshal(task.Results)
	if err != nil {
		resultsJSON = []byte("{}")
	}

	_, err = s.db.Exec(`
		INSERT INTO ping_table_tasks (id, created_at, finished_at, latency_type, status, total_pairs, completed_pairs, nodes_json, results_json, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			finished_at = excluded.finished_at,
			latency_type = excluded.latency_type,
			status = excluded.status,
			total_pairs = excluded.total_pairs,
			completed_pairs = excluded.completed_pairs,
			nodes_json = excluded.nodes_json,
			results_json = excluded.results_json,
			error = excluded.error
	`, task.ID, task.CreatedAt, task.FinishedAt, task.LatencyType, task.Status, task.TotalPairs, task.CompletedPairs, string(nodesJSON), string(resultsJSON), task.Error)

	return err
}

// GetPingTableTask retrieves a specific ping table task by ID
func (s *StateStore) GetPingTableTask(id string) (*PingTableTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.db == nil {
		return nil, errors.New("database not initialized")
	}

	var (
		task        PingTableTask
		nodesJSON   string
		resultsJSON string
	)

	err := s.db.QueryRow(`
		SELECT id, created_at, finished_at, latency_type, status, total_pairs, completed_pairs, nodes_json, results_json, error
		FROM ping_table_tasks
		WHERE id = ?
	`, id).Scan(
		&task.ID,
		&task.CreatedAt,
		&task.FinishedAt,
		&task.LatencyType,
		&task.Status,
		&task.TotalPairs,
		&task.CompletedPairs,
		&nodesJSON,
		&resultsJSON,
		&task.Error,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if nodesJSON != "" {
		_ = json.Unmarshal([]byte(nodesJSON), &task.Nodes)
	}
	if task.Nodes == nil {
		task.Nodes = []string{}
	}

	if resultsJSON != "" {
		_ = json.Unmarshal([]byte(resultsJSON), &task.Results)
	}
	if task.Results == nil {
		task.Results = make(map[string]map[string]*PingTableItem)
	}

	return &task, nil
}

// ListPingTableTasks returns a list of ping table task summaries ordered by created_at DESC
func (s *StateStore) ListPingTableTasks(limit int) ([]PingTableTaskSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.db == nil {
		return []PingTableTaskSummary{}, nil
	}

	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.Query(`
		SELECT id, created_at, finished_at, latency_type, status, total_pairs, completed_pairs, nodes_json, error
		FROM ping_table_tasks
		ORDER BY created_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []PingTableTaskSummary
	for rows.Next() {
		var (
			sum       PingTableTaskSummary
			nodesJSON string
		)
		if err := rows.Scan(
			&sum.ID,
			&sum.CreatedAt,
			&sum.FinishedAt,
			&sum.LatencyType,
			&sum.Status,
			&sum.TotalPairs,
			&sum.CompletedPairs,
			&nodesJSON,
			&sum.Error,
		); err == nil {
			var nodes []string
			if nodesJSON != "" {
				_ = json.Unmarshal([]byte(nodesJSON), &nodes)
			}
			sum.NodeCount = len(nodes)
			summaries = append(summaries, sum)
		}
	}

	if summaries == nil {
		summaries = []PingTableTaskSummary{}
	}

	return summaries, nil
}

// DeletePingTableTask removes a ping table task from database
func (s *StateStore) DeletePingTableTask(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return nil
	}

	_, err := s.db.Exec("DELETE FROM ping_table_tasks WHERE id = ?", id)
	return err
}
