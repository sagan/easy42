package config

import "testing"

// TestSelectorSemantics locks in the shared selector rules used by every feature
// (workload apps/scripts, cron tasks, Device Helper tasks and SSH key rules):
//
//   - {} / nil matches no node
//   - {"all": "1"} is the only way to match every node
//   - any other "all" value or a combination with other keys is invalid
func TestSelectorSemantics(t *testing.T) {
	node := Node{Name: "edge-1", Tags: []string{"edge"}, IP: "192.168.100.1/24"}

	if NodeMatchesSelector(node, nil) {
		t.Fatal("nil selector must match no node")
	}
	if NodeMatchesSelector(node, map[string]string{}) {
		t.Fatal("empty selector must match no node")
	}
	if !NodeMatchesSelector(node, AllNodesSelector()) {
		t.Fatal("all-nodes selector must match every node")
	}
	if NodeMatchesSelector(node, map[string]string{"all": "true"}) {
		t.Fatal(`legacy {"all": "true"} must not match (only "1" is the all-nodes marker)`)
	}
	if NodeMatchesSelector(node, map[string]string{"all": "1", "tag": "edge"}) {
		t.Fatal("all-nodes marker combined with other keys must match nothing")
	}

	if AppTargetsNode(App{}, &node, "") || CronTaskTargetsNode(CronTask{}, &node, "") {
		t.Fatal("apps and cron tasks without a selector must target no node")
	}
	if !AppTargetsNode(App{Selector: AllNodesSelector()}, &node, "") {
		t.Fatal("apps with the all-nodes selector must target every node")
	}
	if !CronTaskTargetsNode(CronTask{Selector: AllNodesSelector()}, &node, "") {
		t.Fatal("cron tasks with the all-nodes selector must target every node")
	}

	valid := []map[string]string{
		nil,
		{},
		{"name": "edge-1"},
		{"tag": "edge,core"},
		{"env:NODE_IP": "192.168.100.1"},
		{"all": "1"},
	}
	for _, selector := range valid {
		if err := ValidateSelector(selector); err != nil {
			t.Fatalf("expected %v to be valid: %v", selector, err)
		}
	}
	invalid := []map[string]string{
		{"all": "true"},
		{"all": "1", "tag": "edge"},
		{"bogus": "x"},
		{"env:NODE_IP": "1,2"},
		{"env:": "x"},
	}
	for _, selector := range invalid {
		if err := ValidateSelector(selector); err == nil {
			t.Fatalf("expected %v to be rejected", selector)
		}
	}

	if !IsAllNodesSelector(AllNodesSelector()) || IsAllNodesSelector(map[string]string{"all": "true"}) {
		t.Fatal("IsAllNodesSelector must accept only {\"all\": \"1\"} exactly")
	}
}
