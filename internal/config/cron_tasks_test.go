package config

import (
	"testing"
)

func TestValidateCronExpr(t *testing.T) {
	valid := []string{
		"0 */1 * * *",
		"*/1 * * * *",
		"*/5 * * * *",
		"0 0 * * *",
		"15 10 * * *",
		"0,15,30,45 * * * *",
		"0 9-17 * * 1-5",
		"0 0 1 1 *",
		"*/30 * * * * *", // 6 fields (with seconds)
		"0 0 12 * * ?",
	}

	for _, expr := range valid {
		if err := ValidateCronExpr(expr); err != nil {
			t.Errorf("expected valid for %q, got error: %v", expr, err)
		}
	}

	invalid := []string{
		"",
		"* * *",
		"* * * * * * *", // 7 fields
		"60 * * * *",    // min 60 out of bounds
		"* 25 * * *",    // hour 25 out of bounds
		"* * 32 * *",    // dom 32 out of bounds
		"* * * 13 *",    // mon 13 out of bounds
		"* * * * 8",     // dow 8 out of bounds
		"*/0 * * * *",   // step 0 invalid
		"abc * * * *",
	}

	for _, expr := range invalid {
		if err := ValidateCronExpr(expr); err == nil {
			t.Errorf("expected error for invalid expression %q, but got nil", expr)
		}
	}
}

func TestCronTaskTargetsNode(t *testing.T) {
	node1 := &Node{Name: "node-tokyo", Tags: []string{"edge", "prod"}}
	node2 := &Node{Name: "node-fra", Tags: []string{"core"}}

	manualTask := CronTask{Selector: map[string]string{"name": "node-tokyo"}}
	if !CronTaskTargetsNode(manualTask, node1, "") {
		t.Errorf("expected manual task to target node-tokyo")
	}
	if CronTaskTargetsNode(manualTask, node2, "") {
		t.Errorf("expected manual task not to target node-fra")
	}

	// An empty selector matches every node.
	allTask := CronTask{}
	if !CronTaskTargetsNode(allTask, node1, "") || !CronTaskTargetsNode(allTask, node2, "") {
		t.Errorf("expected empty selector to match every node")
	}

	// A name selector with an empty value matches no node.
	noneTask := CronTask{Selector: map[string]string{"name": ""}}
	if CronTaskTargetsNode(noneTask, node1, "") {
		t.Errorf("expected empty name selector to match no node")
	}

	tagTask := CronTask{Selector: map[string]string{"tag": "edge"}}
	if !CronTaskTargetsNode(tagTask, node1, "") {
		t.Errorf("expected tag task to target node-tokyo")
	}
	if CronTaskTargetsNode(tagTask, node2, "") {
		t.Errorf("expected tag task not to target node-fra")
	}

	// Name-only selectors work even when only the node name is known.
	if !CronTaskTargetsNode(manualTask, nil, "node-tokyo") {
		t.Errorf("expected manual task to target node-tokyo by name only")
	}
	if CronTaskTargetsNode(tagTask, nil, "node-tokyo") {
		t.Errorf("expected tag task not to target node-tokyo without node definition")
	}
}

func TestValidateCronTaskSelector(t *testing.T) {
	if err := ValidateCronTask(CronTask{}); err != nil {
		t.Errorf("expected empty selector to be valid, got: %v", err)
	}
	if err := ValidateCronTask(CronTask{Selector: map[string]string{"tag": "edge"}}); err != nil {
		t.Errorf("expected tag selector to be valid, got: %v", err)
	}
	if err := ValidateCronTask(CronTask{Selector: map[string]string{"bogus": "value"}}); err == nil {
		t.Errorf("expected error with unknown selector key")
	}
	if err := ValidateCronTask(CronTask{Selector: map[string]string{"env:": "value"}}); err == nil {
		t.Errorf("expected error with empty env selector variable name")
	}
}
