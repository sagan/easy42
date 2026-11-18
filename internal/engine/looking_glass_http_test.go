package engine

import (
	"os"
	"strings"
	"testing"

	"easy42/internal/config"
	"easy42/internal/lookingglass"
)

func TestResolveHTTPParams(t *testing.T) {
	// Params take precedence over the interpolated command
	target, count, err := resolveHTTPParams("http https://example.com/ 1", map[string]string{
		"target": "https://example.com/api",
		"count":  "3",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target != "https://example.com/api" || count != 3 {
		t.Errorf("unexpected result: target=%q count=%d", target, count)
	}

	// Count is capped at the HTTP maximum
	if _, count, err := resolveHTTPParams("http https://example.com/ 100", map[string]string{
		"target": "https://example.com/",
		"count":  "100",
	}); err != nil || count != config.HTTPProbeMaxCount {
		t.Errorf("expected count capped to %d, got %d (err=%v)", config.HTTPProbeMaxCount, count, err)
	}

	// Ad-hoc style command without params
	target, count, err = resolveHTTPParams("http http://1.1.1.1:8080/health 2", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target != "http://1.1.1.1:8080/health" || count != 2 {
		t.Errorf("unexpected result: target=%q count=%d", target, count)
	}

	// Default count when neither params nor command specify it
	if _, count, err := resolveHTTPParams("http https://example.com/", nil); err != nil || count != 1 {
		t.Errorf("expected default count 1, got %d (err=%v)", count, err)
	}

	// Invalid URLs are rejected
	for _, c := range []string{"http example.com", "http ftp://example.com"} {
		if _, _, err := resolveHTTPParams(c, nil); err == nil {
			t.Errorf("expected %q to be rejected", c)
		}
	}
	if _, _, err := resolveHTTPParams("", map[string]string{"target": "nope"}); err == nil {
		t.Errorf("expected invalid target param to be rejected")
	}
}

func TestHTTPBuiltinTaskDefinition(t *testing.T) {
	var httpTask *config.LookingGlassTask
	for _, task := range config.DefaultLookingGlassTasks() {
		if task.ID == "http" {
			task := task
			httpTask = &task
			break
		}
	}
	if httpTask == nil {
		t.Fatal("expected an 'http' built-in Looking Glass task")
	}
	if httpTask.Parser != "http" {
		t.Errorf("expected parser 'http', got %q", httpTask.Parser)
	}
	if len(httpTask.Params) != 2 {
		t.Fatalf("expected 2 params (target, count), got %d", len(httpTask.Params))
	}

	var targetParam, countParam *config.TaskParam
	for i := range httpTask.Params {
		switch httpTask.Params[i].Key {
		case "target":
			targetParam = &httpTask.Params[i]
		case "count":
			countParam = &httpTask.Params[i]
		}
	}
	if targetParam == nil || targetParam.Type != config.ParamTypeURL || !targetParam.Required {
		t.Errorf("expected a required URL 'target' param, got %+v", targetParam)
	}
	if countParam == nil {
		t.Fatal("expected a 'count' param")
	}
	if countParam.DefaultValue != "1" {
		t.Errorf("expected HTTP count default to be 1, got %q", countParam.DefaultValue)
	}
	if !strings.Contains(countParam.Description, "1-5") {
		t.Errorf("expected HTTP count description to mention the 1-%d range, got %q", config.HTTPProbeMaxCount, countParam.Description)
	}

	// The built-in template resolves to a runnable command
	cmd, err := lookingglass.BuildCommand(httpTask.CommandTmpl, httpTask.Params, nil)
	if err != nil {
		t.Fatalf("failed to build default HTTP command: %v", err)
	}
	if got := strings.Split(cmd, " "); got[0] != "http" || got[len(got)-1] != "1" {
		t.Errorf("unexpected default HTTP command: %q", cmd)
	}
}

func TestSaveHTTPCronTask(t *testing.T) {
	mgr, tmpDir := setupTestManager(t)
	defer os.RemoveAll(tmpDir)

	// HTTP task accepted, count defaults to 1 and target URL is kept intact
	saved, err := mgr.SaveCronTask(config.CronTask{
		Name:     "HTTP health check",
		TaskType: "http",
		CronExpr: "*/5 * * * *",
		// Explicit selector to avoid depending on other nodes
		Selector: map[string]string{"name": "foobar"},
		Params:   map[string]string{"target": "https://example.com/api?x=1"},
	})
	if err != nil {
		t.Fatalf("SaveCronTask failed: %v", err)
	}
	if saved.Params["count"] != "1" {
		t.Errorf("expected default http count 1, got %q", saved.Params["count"])
	}
	if saved.Params["target"] != "https://example.com/api?x=1" {
		t.Errorf("unexpected target: %q", saved.Params["target"])
	}

	// Out-of-range counts are reset to the HTTP default
	overCap, err := mgr.SaveCronTask(config.CronTask{
		Name:     "HTTP over cap",
		TaskType: "http",
		CronExpr: "*/5 * * * *",
		Selector: map[string]string{"name": "foobar"},
		Params:   map[string]string{"target": "https://example.com/", "count": "20"},
	})
	if err != nil {
		t.Fatalf("SaveCronTask failed: %v", err)
	}
	if overCap.Params["count"] != "1" {
		t.Errorf("expected over-cap http count reset to 1, got %q", overCap.Params["count"])
	}

	// Non-http tasks keep their default count of 4
	pingTask, err := mgr.SaveCronTask(config.CronTask{
		Name:     "ICMP ping",
		TaskType: "ping",
		CronExpr: "*/5 * * * *",
		Selector: map[string]string{"name": "foobar"},
		Params:   map[string]string{"target": "1.1.1.1"},
	})
	if err != nil {
		t.Fatalf("SaveCronTask failed: %v", err)
	}
	if pingTask.Params["count"] != "4" {
		t.Errorf("expected ping default count 4, got %q", pingTask.Params["count"])
	}

	// Invalid URL is rejected
	if _, err := mgr.SaveCronTask(config.CronTask{
		Name:     "Bad HTTP",
		TaskType: "http",
		CronExpr: "*/5 * * * *",
		Selector: map[string]string{"name": "foobar"},
		Params:   map[string]string{"target": "example.com"},
	}); err == nil {
		t.Errorf("expected invalid http target URL to be rejected")
	}
}

func TestPopulateHTTPCronTaskMetrics(t *testing.T) {
	record := &config.CronTaskResultRecord{
		TaskType: "http",
		Target:   "https://example.com/",
		RawOutput: `{"target":"https://example.com/","packets_sent":2,"packets_received":1,"packet_loss_pct":50,` +
			`"min_ttfb_ms":20,"avg_ttfb_ms":20,"max_ttfb_ms":20,"mdev_ttfb_ms":0,"status_codes":[503],` +
			`"probes":[{"status_code":503,"status":"503 Service Unavailable","ttfb_ms":20},` +
			`{"status_code":0,"status":"","ttfb_ms":0,"error":"request failed: timeout"}]}`,
	}

	PopulateCronTaskParsedMetrics(record)

	if record.PacketsSent != 2 || record.PacketsReceived != 1 {
		t.Errorf("unexpected packet counts: sent=%d recv=%d", record.PacketsSent, record.PacketsReceived)
	}
	if record.PacketLossPct != 50 {
		t.Errorf("expected 50%% loss, got %f", record.PacketLossPct)
	}
	if record.AvgRTTMs != 20 {
		t.Errorf("expected TTFB to populate avg_rtt_ms, got %f", record.AvgRTTMs)
	}
	if !strings.Contains(record.ParsedJSON, `"status_codes":[503]`) {
		t.Errorf("expected parsed JSON to record the status code, got %s", record.ParsedJSON)
	}

	// A 5xx response counts as success (header received), so the task must not be
	// flagged as failed solely because of the status code.
	if isCronTaskFailed(record) {
		t.Errorf("expected a task with a successful response to not be marked failed")
	}

	// All probes failing is a failure
	failed := &config.CronTaskResultRecord{
		TaskType:  "http",
		ExitCode:  1,
		RawOutput: `{"target":"https://example.com/","packets_sent":1,"packets_received":0,"packet_loss_pct":100,"status_codes":[],"probes":[]}`,
	}
	PopulateCronTaskParsedMetrics(failed)
	if !isCronTaskFailed(failed) {
		t.Errorf("expected a task with no successful responses to be marked failed")
	}
}
