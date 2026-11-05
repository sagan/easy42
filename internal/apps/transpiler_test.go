package apps

import (
	"strings"
	"testing"
)

func TestParseSystemdService(t *testing.T) {
	unitContent := `[Unit]
Description=Test App Daemon
After=network.target

[Service]
Type=simple
User=appuser
Group=appgroup
WorkingDirectory=/opt/testapp
ExecStart=/usr/bin/testapp --config /etc/testapp.json
ExecStop=/usr/bin/testapp --stop
Restart=always
Environment="FOO=bar"
EnvironmentFile=/etc/default/testapp

[Install]
WantedBy=multi-user.target
`

	unit, err := ParseSystemdService(unitContent)
	if err != nil {
		t.Fatalf("unexpected error parsing unit: %v", err)
	}

	if unit.Description != "Test App Daemon" {
		t.Errorf("expected Description 'Test App Daemon', got '%s'", unit.Description)
	}
	if unit.ExecStart != "/usr/bin/testapp --config /etc/testapp.json" {
		t.Errorf("expected ExecStart, got '%s'", unit.ExecStart)
	}
	if unit.User != "appuser" || unit.Group != "appgroup" {
		t.Errorf("expected User appuser Group appgroup, got %s:%s", unit.User, unit.Group)
	}
	if unit.WorkingDirectory != "/opt/testapp" {
		t.Errorf("expected WorkingDirectory '/opt/testapp', got '%s'", unit.WorkingDirectory)
	}
	if unit.Restart != "always" {
		t.Errorf("expected Restart 'always', got '%s'", unit.Restart)
	}
	if unit.Environment["FOO"] != "bar" {
		t.Errorf("expected Environment FOO=bar, got '%s'", unit.Environment["FOO"])
	}
}

func TestTranspileToOpenRC(t *testing.T) {
	unit := &ParsedServiceUnit{
		Description:      "My Service",
		ExecStart:        "/usr/local/bin/mydaemon --port 9000",
		User:             "daemon",
		WorkingDirectory: "/var/run",
	}

	openrc, err := TranspileToOpenRC("myservice", unit)
	if err != nil {
		t.Fatalf("unexpected error transpiling to OpenRC: %v", err)
	}

	if !strings.Contains(openrc, `command='/usr/local/bin/mydaemon'`) {
		t.Errorf("expected command in OpenRC, got:\n%s", openrc)
	}
	if !strings.Contains(openrc, `command_args="'--port' '9000'"`) {
		t.Errorf("expected command_args in OpenRC, got:\n%s", openrc)
	}
	if !strings.Contains(openrc, `command_user='daemon'`) {
		t.Errorf("expected command_user in OpenRC, got:\n%s", openrc)
	}
	if !strings.Contains(openrc, `directory='/var/run'`) {
		t.Errorf("expected directory in OpenRC, got:\n%s", openrc)
	}
}

func TestTranspileToProcd(t *testing.T) {
	unit := &ParsedServiceUnit{
		Description: "OpenWrt App",
		ExecStart:   "/bin/app --verbose",
		User:        "root",
		Restart:     "always",
	}
	env := map[string]string{
		"PORT": "8080",
		"KEY":  "value with spaces",
	}

	procd, err := TranspileToProcd("mywrtapp", unit, env)
	if err != nil {
		t.Fatalf("unexpected error transpiling to Procd: %v", err)
	}

	if !strings.Contains(procd, `USE_PROCD=1`) {
		t.Errorf("expected USE_PROCD=1 in Procd, got:\n%s", procd)
	}
	if !strings.Contains(procd, `procd_open_instance "ezapp-mywrtapp"`) {
		t.Errorf("expected procd_open_instance in Procd, got:\n%s", procd)
	}
	if !strings.Contains(procd, `procd_set_param respawn`) {
		t.Errorf("expected respawn in Procd, got:\n%s", procd)
	}
	if !strings.Contains(procd, `PORT='8080'`) {
		t.Errorf("expected env PORT in Procd, got:\n%s", procd)
	}
}

func TestParseEnvFile(t *testing.T) {
	raw := `
# Comment
PORT=8080
LOG_LEVEL="debug"
SECRET='my-secret'
export ADDR=127.0.0.1
`
	res := ParseEnvFile(raw)
	if res["PORT"] != "8080" {
		t.Errorf("expected PORT=8080, got '%s'", res["PORT"])
	}
	if res["LOG_LEVEL"] != "debug" {
		t.Errorf("expected LOG_LEVEL=debug, got '%s'", res["LOG_LEVEL"])
	}
	if res["SECRET"] != "my-secret" {
		t.Errorf("expected SECRET=my-secret, got '%s'", res["SECRET"])
	}
	if res["ADDR"] != "127.0.0.1" {
		t.Errorf("expected ADDR=127.0.0.1, got '%s'", res["ADDR"])
	}
}

func TestEnsureSystemdEnvironmentFile(t *testing.T) {
	// 1. Missing EnvironmentFile (like in rclone issue reported by user)
	rcloneService := `[Unit]
Description=rclone service
After=network.target

[Service]
Type=simple
User=nobody
WorkingDirectory=/var/log
ExecStart=/bin/sh -c "exec rclone serve webdav -L --read-only --user root /root"
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target`

	ensured := EnsureSystemdEnvironmentFile("rclone", rcloneService)
	if !strings.Contains(ensured, "EnvironmentFile=/etc/systemd/system/ezapp-rclone.env") {
		t.Fatalf("expected EnvironmentFile=/etc/systemd/system/ezapp-rclone.env, got:\n%s", ensured)
	}
	if !strings.Contains(ensured, "[Service]\nEnvironmentFile=/etc/systemd/system/ezapp-rclone.env\nType=simple") {
		t.Fatalf("expected EnvironmentFile under [Service], got:\n%s", ensured)
	}

	// 2. Sample service unit with ezapp-sample.env
	sampleService := `[Unit]
Description=My Service
[Service]
EnvironmentFile=/etc/systemd/system/ezapp-sample.env
ExecStart=/usr/bin/mybin`

	ensuredSample := EnsureSystemdEnvironmentFile("mydaemon", sampleService)
	if !strings.Contains(ensuredSample, "EnvironmentFile=/etc/systemd/system/ezapp-mydaemon.env") {
		t.Fatalf("expected ezapp-sample.env to be replaced by ezapp-mydaemon.env, got:\n%s", ensuredSample)
	}
	if strings.Contains(ensuredSample, "ezapp-sample.env") {
		t.Fatalf("expected ezapp-sample.env to no longer exist in content, got:\n%s", ensuredSample)
	}

	// 3. Already containing the expected directive
	alreadyGood := `[Unit]
Description=My Service
[Service]
EnvironmentFile=/etc/systemd/system/ezapp-mydaemon.env
ExecStart=/usr/bin/mybin`

	ensuredGood := EnsureSystemdEnvironmentFile("mydaemon", alreadyGood)
	if ensuredGood != alreadyGood {
		t.Fatalf("expected unchanged content when already present, got:\n%s", ensuredGood)
	}

	// 4. Custom existing EnvironmentFile
	customService := `[Unit]
Description=Custom Service
[Service]
EnvironmentFile=/etc/default/custom
ExecStart=/usr/bin/custom`

	ensuredCustom := EnsureSystemdEnvironmentFile("custom", customService)
	if !strings.Contains(ensuredCustom, "EnvironmentFile=/etc/systemd/system/ezapp-custom.env") {
		t.Fatalf("expected EnvironmentFile=/etc/systemd/system/ezapp-custom.env to be injected, got:\n%s", ensuredCustom)
	}
	if !strings.Contains(ensuredCustom, "EnvironmentFile=/etc/default/custom") {
		t.Fatalf("expected existing custom EnvironmentFile to be preserved, got:\n%s", ensuredCustom)
	}

	// 5. No [Service] section
	noService := `[Unit]
Description=Minimal`
	ensuredNoService := EnsureSystemdEnvironmentFile("minimal", noService)
	if !strings.Contains(ensuredNoService, "[Service]\nEnvironmentFile=/etc/systemd/system/ezapp-minimal.env") {
		t.Fatalf("expected [Service] and EnvironmentFile to be appended, got:\n%s", ensuredNoService)
	}
}

