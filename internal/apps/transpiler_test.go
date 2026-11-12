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
User=root
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

const fullUnit = `[Unit]
Description=Full Service
After=network.target dbus.service
Wants=other.service
Requires=req.service
Before=later.service

[Service]
Type=simple
User=appuser
Group=appgroup
WorkingDirectory=/opt/app
ExecStartPre=/bin/mkdir -p /run/app
ExecStartPre=-/bin/touch /run/app/pre-ok
ExecCondition=/usr/bin/test -f /etc/app.conf
ExecStart=/usr/bin/app --serve
ExecStartPost=/usr/bin/app --notify-ready
ExecStop=/usr/bin/app --stop
ExecStopPost=/bin/rm -f /run/app/done
ExecReload=/usr/bin/app --reload
Restart=always
RestartSec=3s
TimeoutStopSec=15
KillSignal=SIGTERM
StandardOutput=append:/var/log/app.log
StandardError=journal
UMask=0027
Nice=-5
LimitNOFILE=4096
NoNewPrivileges=true
Environment="FOO=bar" "BAZ=qux"
Environment=EMPTY=

[Install]
WantedBy=multi-user.target
`

func mustParse(t *testing.T, content string) *ParsedServiceUnit {
	t.Helper()
	unit, err := ParseSystemdService(content)
	if err != nil {
		t.Fatalf("unexpected error parsing unit: %v", err)
	}
	return unit
}

func TestParseExtendedDirectives(t *testing.T) {
	unit := mustParse(t, fullUnit)

	if unit.Type != "simple" {
		t.Errorf("expected Type simple, got %q", unit.Type)
	}
	if len(unit.ExecStartPre) != 2 {
		t.Errorf("expected 2 ExecStartPre, got %v", unit.ExecStartPre)
	}
	if len(unit.ExecCondition) != 1 || unit.ExecCondition[0] != "/usr/bin/test -f /etc/app.conf" {
		t.Errorf("unexpected ExecCondition %v", unit.ExecCondition)
	}
	if len(unit.ExecStop) != 1 || len(unit.ExecStopPost) != 1 || len(unit.ExecReload) != 1 || len(unit.ExecStartPost) != 1 {
		t.Errorf("unexpected hooks: stop=%v stoppost=%v reload=%v post=%v",
			unit.ExecStop, unit.ExecStopPost, unit.ExecReload, unit.ExecStartPost)
	}
	if unit.RestartSec != "3s" || unit.TimeoutStopSec != "15" || unit.KillSignal != "SIGTERM" {
		t.Errorf("unexpected restart/timeout fields: %q %q %q", unit.RestartSec, unit.TimeoutStopSec, unit.KillSignal)
	}
	if unit.StandardOutput != "append:/var/log/app.log" || unit.StandardError != "journal" {
		t.Errorf("unexpected stdio: %q %q", unit.StandardOutput, unit.StandardError)
	}
	if unit.UMask != "0027" || unit.Nice != "-5" || unit.LimitNOFILE != "4096" || !unit.NoNewPrivileges {
		t.Errorf("unexpected misc fields: umask=%q nice=%q nofile=%q nnp=%v", unit.UMask, unit.Nice, unit.LimitNOFILE, unit.NoNewPrivileges)
	}
	if unit.Environment["FOO"] != "bar" || unit.Environment["BAZ"] != "qux" || unit.Environment["EMPTY"] != "" {
		t.Errorf("unexpected environment %v", unit.Environment)
	}
	if len(unit.After) != 2 || len(unit.Wants) != 1 || len(unit.Requires) != 1 || len(unit.Before) != 1 {
		t.Errorf("unexpected deps after=%v wants=%v requires=%v before=%v", unit.After, unit.Wants, unit.Requires, unit.Before)
	}
	if len(unit.WantedBy) != 1 || unit.WantedBy[0] != "multi-user.target" {
		t.Errorf("unexpected WantedBy %v", unit.WantedBy)
	}
}

func TestParseListResetAndContinuation(t *testing.T) {
	unit := mustParse(t, `[Service]
ExecStartPre=/bin/first
ExecStartPre=
ExecStartPre=/bin/second \
  --with-arg
ExecStart=/bin/main
`)
	if len(unit.ExecStartPre) != 1 || unit.ExecStartPre[0] != "/bin/second --with-arg" {
		t.Errorf("expected reset+continuation to yield one pre command, got %v", unit.ExecStartPre)
	}
}

func TestParseDurationSeconds(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"5", 5, true},
		{"500ms", 0.5, true},
		{"2min", 120, true},
		{"1h 30min", 5400, true},
		{"1min 30s", 90, true},
		{"infinity", 0, false},
		{"abc", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, ok := ParseDurationSeconds(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("ParseDurationSeconds(%q) = %v, %v; want %v, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestTranspileExtendedToOpenRC(t *testing.T) {
	unit := mustParse(t, fullUnit)
	openrc, err := TranspileToOpenRC("full", unit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{
		`supervisor="supervise-daemon"`,
		`respawn_delay="3"`,
		`command_user='appuser:appgroup'`,
		`directory='/opt/app'`,
		`umask='0027'`,
		`no_new_privs="yes"`,
		`start_stop_daemon_args="--nice -5"`,
		`stopsig='SIGTERM'`,
		`retry="TERM/15/KILL/5"`,
		`output_log='/var/log/app.log'`,
		`error_logger="logger -t ezapp-full"`,
		"after dbus",
		"use other",
		"before later",
		"/usr/bin/test -f /etc/app.conf || return 1",
		"/bin/mkdir -p /run/app || return 1",
		"/bin/touch /run/app/pre-ok || true",
		"/usr/bin/app --notify-ready || return 1",
		"/usr/bin/app --stop || true",
		"/bin/rm -f /run/app/done || true",
		"/usr/bin/app --reload || return 1",
		`extra_started_commands="reload"`,
		"ulimit -n 4096",
	} {
		if !strings.Contains(openrc, want) {
			t.Errorf("expected OpenRC script to contain %q, got:\n%s", want, openrc)
		}
	}
}

func TestTranspileExtendedToProcd(t *testing.T) {
	unit := mustParse(t, fullUnit)
	procd, err := TranspileToProcd("full", unit, map[string]string{"PORT": "8080"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{
		"/usr/bin/test -f /etc/app.conf || return 1",
		"/bin/mkdir -p /run/app || return 1",
		"procd_set_param respawn 3600 3 5",
		"procd_set_param user 'appuser'",
		"procd_set_param group 'appgroup'",
		"procd_set_param nice -5",
		"procd_set_param term_timeout 15",
		"procd_set_param no_new_privs 1",
		`procd_set_param limits nofile="4096"`,
		"procd_set_param stderr 1",
		"stop_service() {",
		"/usr/bin/app --stop || true",
		"reload_service() {",
		"/usr/bin/app --reload || return 1",
		"cd ",
		"/opt/app",
		"umask 0027",
		"exec ",
		"/usr/bin/app",
		"--serve",
		">> ",
		"/var/log/app.log",
	} {
		if !strings.Contains(procd, want) {
			t.Errorf("expected Procd script to contain %q, got:\n%s", want, procd)
		}
	}
	// WorkingDirectory is only expressible via the shell wrapper on procd.
	if !strings.Contains(procd, `'/bin/sh' '-c'`) {
		t.Errorf("expected shell wrapper command for WorkingDirectory, got:\n%s", procd)
	}
}

func TestTranspileOneshotOpenRC(t *testing.T) {
	unit := mustParse(t, `[Service]
Type=oneshot
ExecStart=/bin/first
ExecStart=/bin/second
Restart=always
`)
	openrc, err := TranspileToOpenRC("shot", unit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(openrc, `command_background="no"`) {
		t.Errorf("expected oneshot to run in foreground, got:\n%s", openrc)
	}
	// Earlier ExecStart lines of a oneshot run sequentially in start_pre.
	if !strings.Contains(openrc, "/bin/first || return 1") {
		t.Errorf("expected first oneshot ExecStart in start_pre, got:\n%s", openrc)
	}
	if strings.Contains(openrc, "supervisor=") {
		t.Errorf("oneshot must not be supervised, got:\n%s", openrc)
	}
}

func TestUnsupportedDirectiveNotes(t *testing.T) {
	unit := mustParse(t, `[Service]
ExecStart=/bin/main
PrivateTmp=true
ProtectSystem=strict
TimeoutStartSec=30
`)
	procd, err := TranspileToProcd("sandbox", unit, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"Service.PrivateTmp", "Service.ProtectSystem", "Service.TimeoutStartSec"} {
		if !strings.Contains(procd, want) {
			t.Errorf("expected dropped-directive note for %s, got:\n%s", want, procd)
		}
	}
	openrc, err := TranspileToOpenRC("sandbox", unit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(openrc, "Service.PrivateTmp") {
		t.Errorf("expected dropped-directive note in OpenRC script, got:\n%s", openrc)
	}
}

func TestExecPrefixHandling(t *testing.T) {
	unit := mustParse(t, `[Service]
ExecStart=-/usr/bin/app --serve
`)
	openrc, err := TranspileToOpenRC("prefixed", unit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(openrc, `command='/usr/bin/app'`) {
		t.Errorf("expected '-' prefix to be stripped, got:\n%s", openrc)
	}
}

func TestParseInitSystem(t *testing.T) {
	cases := map[string]InitSystem{
		"systemd": InitSystemSystemd,
		"SYSTEMD": InitSystemSystemd,
		" procd ": InitSystemProcd,
		"openwrt": InitSystemProcd,
		"openrc":  InitSystemOpenRC,
		"alpine":  InitSystemOpenRC,
		"":        "",
		"upstart": "",
	}
	for in, want := range cases {
		if got := ParseInitSystem(in); got != want {
			t.Errorf("ParseInitSystem(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestDetectInitSystem covers the regression that broke OpenWrt app deployment:
// agents report os_info as "<os> <arch>" (e.g. "linux mips"), which never
// contains "openwrt", so the legacy heuristic classified OpenWrt nodes as
// systemd and skipped procd transpilation entirely.
func TestDetectInitSystem(t *testing.T) {
	cases := []struct {
		name     string
		reported string
		osInfo   string
		want     InitSystem
	}{
		// The reported field always wins.
		{"openwrt via reported field", "procd", "linux mips", InitSystemProcd},
		{"alpine via reported field", "openrc", "linux x86_64", InitSystemOpenRC},
		{"systemd via reported field", "systemd", "linux x86_64", InitSystemSystemd},
		// Legacy agents: fall back to the os_info heuristic.
		{"legacy openwrt os_info", "", "linux x86_64 openwrt", InitSystemProcd},
		{"legacy alpine os_info", "", "linux x86_64 alpine", InitSystemOpenRC},
		{"legacy plain linux", "", "linux amd64", InitSystemSystemd},
		// The old broken case: a real OpenWrt host with a legacy agent is
		// misdetected as systemd. This documents the limitation, which is why
		// the agent now reports init_system explicitly.
		{"legacy openwrt without marker", "", "linux mips", InitSystemSystemd},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DetectInitSystem(c.reported, c.osInfo); got != c.want {
				t.Errorf("DetectInitSystem(%q, %q) = %q, want %q", c.reported, c.osInfo, got, c.want)
			}
		})
	}
}

// TestTranspileWgWatcherUnitToProcd reproduces the wg-watcher app definition
// from the bug report and asserts the generated script is a valid procd init
// script rather than a raw systemd unit.
func TestTranspileWgWatcherUnitToProcd(t *testing.T) {
	unit := "[Unit]\nDescription=wg-watcher\nAfter=network.target\n\n[Service]\nType=simple\nUser=root\nWorkingDirectory=/var/log\nEnvironmentFile=/etc/systemd/system/ezapp-wg-watcher.env\nExecStart=/bin/sh -c \"exec wg-watcher\"\nRestart=always\nRestartSec=5\n\n[Install]\nWantedBy=multi-user.target\n"

	parsed, err := ParseSystemdService(unit)
	if err != nil {
		t.Fatalf("ParseSystemdService failed: %v", err)
	}
	env := ParseEnvFile("DIR=/etc/ezapp/wg-watcher\nIP=192.168.50.1\n")
	for k, v := range parsed.Environment {
		if _, ok := env[k]; !ok {
			env[k] = v
		}
	}

	script, err := TranspileToProcd("wg-watcher", parsed, env)
	if err != nil {
		t.Fatalf("TranspileToProcd failed: %v", err)
	}

	if !strings.HasPrefix(script, "#!/bin/sh /etc/rc.common") {
		t.Errorf("script must start with the rc.common shebang, got:\n%s", script)
	}
	for _, want := range []string{
		"USE_PROCD=1",
		"start_service()",
		"procd_set_param command",
		"procd_set_param respawn",
		"procd_set_param env",
		"procd_close_instance",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("script missing %q:\n%s", want, script)
		}
	}
	// The raw systemd section headers must not leak into the init script.
	for _, unwanted := range []string{"[Unit]", "[Service]", "[Install]"} {
		if strings.Contains(script, unwanted) {
			t.Errorf("script must not contain %q:\n%s", unwanted, script)
		}
	}
}
