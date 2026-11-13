package apps

import (
	"bufio"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// InitSystem identifies the service manager running on a node.
type InitSystem string

const (
	// InitSystemSystemd is the default for Linux hosts.
	InitSystemSystemd InitSystem = "systemd"
	// InitSystemProcd is used by OpenWrt.
	InitSystemProcd InitSystem = "procd"
	// InitSystemOpenRC is used by Alpine and other OpenRC distributions.
	InitSystemOpenRC InitSystem = "openrc"
)

// ParseInitSystem normalizes an init system name reported by an agent.
// It returns "" when the value is empty or unrecognized.
func ParseInitSystem(value string) InitSystem {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "systemd":
		return InitSystemSystemd
	case "procd", "openwrt":
		return InitSystemProcd
	case "openrc", "alpine":
		return InitSystemOpenRC
	default:
		return ""
	}
}

// DetectInitSystem determines the init system of a node from the init system
// reported by its agent, falling back to the legacy os_info heuristic for
// agents that predate the init_system field.
//
// The heuristic is unreliable: agents report os_info as "<os> <arch>", which
// never contains "openwrt", so OpenWrt nodes were previously treated as
// systemd. The reported field is therefore preferred whenever present.
func DetectInitSystem(reportedInitSystem, osInfo string) InitSystem {
	if is := ParseInitSystem(reportedInitSystem); is != "" {
		return is
	}

	lower := strings.ToLower(osInfo)
	switch {
	case strings.Contains(lower, "openwrt"), strings.Contains(lower, "procd"):
		return InitSystemProcd
	case strings.Contains(lower, "alpine"), strings.Contains(lower, "openrc"):
		return InitSystemOpenRC
	default:
		return InitSystemSystemd
	}
}

// ParsedServiceUnit holds the extracted fields from a systemd service unit.
//
// Only directives that have a meaningful equivalent (or a close
// approximation) in OpenRC / procd are transpiled. Directives that are
// parsed but have no equivalent in a target init system are reported as
// dropped in the generated script header (see unsupportedDirectives), so a
// user previewing the transpilation can see what was lost instead of
// silently losing it.
type ParsedServiceUnit struct {
	Description              string
	Type                     string // simple, exec, forking, oneshot, notify, dbus
	ExecStart                string
	ExecStartCommands        []string // every ExecStart line (oneshot may have several)
	ExecStartPre             []string
	ExecStartPost            []string
	ExecStop                 []string
	ExecStopPost             []string
	ExecReload               []string
	ExecCondition            []string
	PIDFile                  string
	WorkingDirectory         string
	User                     string
	Group                    string
	SupplementaryGroups      []string
	Restart                  string
	RestartSec               string
	RestartPreventExitStatus string
	RestartForceExitStatus   string
	SuccessExitStatus        string
	RemainAfterExit          bool
	TimeoutStartSec          string
	TimeoutStopSec           string
	TimeoutSec               string
	KillSignal               string
	StandardInput            string
	StandardOutput           string
	StandardError            string
	UMask                    string
	Nice                     string
	LimitNOFILE              string
	LimitNPROC               string
	LimitCORE                string
	AmbientCapabilities      []string
	NoNewPrivileges          bool
	After                    []string
	Before                   []string
	Wants                    []string
	Requires                 []string
	BindsTo                  []string
	PartOf                   []string
	Conflicts                []string
	WantedBy                 []string
	Environment              map[string]string
	EnvironmentFiles         []string
	RawDirectives            map[string]string
}

// ParseSystemdService parses key systemd service unit directives.
//
// Directives may be repeated (ExecStartPre, Environment, After, ...);
// later occurrences accumulate. An empty assignment (e.g. "ExecStartPre=")
// resets the accumulated list, matching systemd semantics. A trailing "\"
// continues a directive on the next line.
func ParseSystemdService(content string) (*ParsedServiceUnit, error) {
	unit := &ParsedServiceUnit{
		Environment:   make(map[string]string),
		RawDirectives: make(map[string]string),
	}

	scanner := bufio.NewScanner(strings.NewReader(content))
	// Units can contain long ExecStart lines; raise the default 64K limit.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	currentSection := ""

	var pending string
	flush := func(line string) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			return
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.TrimSpace(line[1 : len(line)-1])
			return
		}
		idx := strings.Index(line, "=")
		if idx == -1 {
			return
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		unit.RawDirectives[fmt.Sprintf("%s.%s", currentSection, key)] = val
		unit.applyDirective(currentSection, key, val)
	}

	for scanner.Scan() {
		raw := scanner.Text()
		trimmedRight := strings.TrimRight(raw, " \t\r")
		if strings.HasSuffix(trimmedRight, `\`) {
			pending += strings.TrimSpace(strings.TrimSuffix(trimmedRight, `\`)) + " "
			continue
		}
		line := pending + raw
		if pending != "" {
			line = pending + strings.TrimSpace(raw)
		}
		pending = ""
		flush(line)
	}
	if pending != "" {
		flush(pending)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading systemd unit: %w", err)
	}

	return unit, nil
}

func (u *ParsedServiceUnit) applyDirective(section, key, val string) {
	lowerKey := strings.ToLower(key)

	// List directives: empty value resets the list (systemd semantics).
	appendList := func(dst *[]string) {
		if val == "" {
			*dst = nil
			return
		}
		*dst = append(*dst, splitDirectiveList(val)...)
	}
	// Exec* directives: each line is one whole command, never split.
	appendCommand := func(dst *[]string) {
		if val == "" {
			*dst = nil
			return
		}
		*dst = append(*dst, val)
	}
	setString := func(dst *string) { *dst = val }

	switch lowerKey {
	case "description":
		setString(&u.Description)
	case "type":
		setString(&u.Type)
		u.Type = strings.ToLower(u.Type)
	case "execstart":
		if val == "" {
			u.ExecStart = ""
			u.ExecStartCommands = nil
			return
		}
		u.ExecStart = val
		u.ExecStartCommands = append(u.ExecStartCommands, val)
	case "execstartpre":
		appendCommand(&u.ExecStartPre)
	case "execstartpost":
		appendCommand(&u.ExecStartPost)
	case "execstop":
		appendCommand(&u.ExecStop)
	case "execstoppost":
		appendCommand(&u.ExecStopPost)
	case "execreload":
		appendCommand(&u.ExecReload)
	case "execcondition":
		appendCommand(&u.ExecCondition)
	case "pidfile":
		setString(&u.PIDFile)
	case "workingdirectory":
		setString(&u.WorkingDirectory)
	case "user":
		setString(&u.User)
	case "group":
		setString(&u.Group)
	case "supplementarygroups":
		appendList(&u.SupplementaryGroups)
	case "restart":
		setString(&u.Restart)
		u.Restart = strings.ToLower(u.Restart)
	case "restartsec":
		setString(&u.RestartSec)
	case "restartpreventexitstatus":
		setString(&u.RestartPreventExitStatus)
	case "restartforceexitstatus":
		setString(&u.RestartForceExitStatus)
	case "successexitstatus":
		setString(&u.SuccessExitStatus)
	case "remainafterexit":
		u.RemainAfterExit = parseBool(val)
	case "timeoutstartsec":
		setString(&u.TimeoutStartSec)
	case "timeoutstopsec":
		setString(&u.TimeoutStopSec)
	case "timeoutsec":
		setString(&u.TimeoutSec)
		if u.TimeoutStartSec == "" {
			u.TimeoutStartSec = val
		}
		if u.TimeoutStopSec == "" {
			u.TimeoutStopSec = val
		}
	case "killsignal":
		setString(&u.KillSignal)
	case "standardinput":
		setString(&u.StandardInput)
	case "standardoutput":
		setString(&u.StandardOutput)
	case "standarderror":
		setString(&u.StandardError)
	case "umask":
		setString(&u.UMask)
	case "nice":
		setString(&u.Nice)
	case "limitnofile":
		setString(&u.LimitNOFILE)
	case "limitnproc":
		setString(&u.LimitNPROC)
	case "limitcore":
		setString(&u.LimitCORE)
	case "ambientcapabilities":
		appendList(&u.AmbientCapabilities)
	case "nonewprivileges":
		u.NoNewPrivileges = parseBool(val)
	case "after":
		appendList(&u.After)
	case "before":
		appendList(&u.Before)
	case "wants":
		appendList(&u.Wants)
	case "requires":
		appendList(&u.Requires)
	case "bindsto":
		appendList(&u.BindsTo)
	case "partof":
		appendList(&u.PartOf)
	case "conflicts":
		appendList(&u.Conflicts)
	case "wantedby":
		appendList(&u.WantedBy)
	case "environment":
		if val == "" {
			u.Environment = make(map[string]string)
			return
		}
		parseEnvironmentAssignments(val, u.Environment)
	case "environmentfile":
		if val == "" {
			u.EnvironmentFiles = nil
			return
		}
		// A leading "-" marks the file as optional.
		u.EnvironmentFiles = append(u.EnvironmentFiles, strings.Trim(strings.TrimSpace(strings.TrimPrefix(val, "-")), `"'`))
	}
}

func parseBool(val string) bool {
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "yes", "true", "on", "1":
		return true
	}
	return false
}

// splitDirectiveList splits a space-separated directive value, honouring
// basic quoting (delegated to splitCommand).
func splitDirectiveList(val string) []string {
	return splitCommand(val)
}

// parseEnvironmentAssignments parses one or more Environment= assignments:
// Environment="FOO=bar" , Environment=FOO=bar BAZ=qux , Environment=-FOO (unset).
func parseEnvironmentAssignments(val string, env map[string]string) {
	for _, tok := range splitCommand(val) {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if strings.HasPrefix(tok, "-") {
			key := strings.TrimPrefix(tok, "-")
			if idx := strings.Index(key, "="); idx != -1 {
				key = key[:idx]
			}
			delete(env, key)
			continue
		}
		eqIdx := strings.Index(tok, "=")
		if eqIdx == -1 {
			env[tok] = ""
			continue
		}
		k := strings.TrimSpace(tok[:eqIdx])
		v := strings.TrimSpace(tok[eqIdx+1:])
		if (strings.HasPrefix(v, `"`) && strings.HasSuffix(v, `"`)) ||
			(strings.HasPrefix(v, `'`) && strings.HasSuffix(v, `'`)) {
			if len(v) >= 2 {
				v = v[1 : len(v)-1]
			}
		}
		env[k] = v
	}
}

// ParseEnvFile parses a standard KEY=VALUE .env file
func ParseEnvFile(envContent string) map[string]string {
	res := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(envContent))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(line[7:])
		}
		eqIdx := strings.Index(line, "=")
		if eqIdx == -1 {
			continue
		}
		key := strings.TrimSpace(line[:eqIdx])
		val := strings.TrimSpace(line[eqIdx+1:])
		// Remove wrapping quotes if present
		if (strings.HasPrefix(val, `"`) && strings.HasSuffix(val, `"`)) ||
			(strings.HasPrefix(val, `'`) && strings.HasSuffix(val, `'`)) {
			if len(val) >= 2 {
				val = val[1 : len(val)-1]
			}
		}
		res[key] = val
	}
	return res
}

// ShellEscape quotes a string safely for POSIX shell
func ShellEscape(s string) string {
	// If it contains single quote, replace ' with '\''
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// --- exec command helpers -------------------------------------------------

type execCommand struct {
	raw           string // cleaned command line (prefix characters removed)
	ignoreFailure bool   // "-" prefix
	runAsRoot     bool   // "+" / "!" prefix: run as root, ignore User=
}

// stripExecPrefixes removes systemd Exec* prefix characters (-, +, !, @, :)
// from the start of a command line. Combinations such as "-+" are handled.
func stripExecPrefixes(raw string) execCommand {
	raw = strings.TrimSpace(raw)
	ec := execCommand{}
	i := 0
	for i < len(raw) {
		switch raw[i] {
		case '-':
			ec.ignoreFailure = true
		case '+', '!':
			ec.runAsRoot = true
		case '@', ':':
			// "@": argv[0] override, ":": PATH lookup behaviour - no
			// equivalent in the target init systems, dropped.
		default:
			goto done
		}
		i++
		// "!!" is a single prefix; the loop consumes both '!' separately.
	}
done:
	// If the loop consumed the whole string, keep it (degenerate input).
	if i == 0 || i >= len(raw) {
		ec.raw = raw
		return ec
	}
	ec.raw = strings.TrimSpace(raw[i:])
	return ec
}

// shellHookLine renders an Exec* hook command as a shell line. failMode is
// "return" (abort the hook on failure) or "ignore" (never fail the hook).
func shellHookLine(raw, failMode string) string {
	ec := stripExecPrefixes(raw)
	if ec.raw == "" {
		return ""
	}
	if ec.ignoreFailure || failMode == "ignore" {
		return ec.raw + " || true"
	}
	return ec.raw + " || return 1"
}

// --- systemd duration helpers ---------------------------------------------

var durationPartRegex = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*(usec|us|µs|ms|seconds?|secs?|s|minutes?|mins?|m|hours?|hrs?|h|days?|d|weeks?|w|months?|M|years?|y)?`)

var durationUnits = map[string]float64{
	"": 1, "s": 1, "sec": 1, "secs": 1, "second": 1, "seconds": 1,
	"ms": 0.001, "us": 0.000001, "usec": 0.000001, "µs": 0.000001,
	"m": 60, "min": 60, "mins": 60, "minute": 60, "minutes": 60,
	"h": 3600, "hr": 3600, "hrs": 3600, "hour": 3600, "hours": 3600,
	"d": 86400, "day": 86400, "days": 86400,
	"w": 604800, "week": 604800, "weeks": 604800,
	"M": 2629746, "month": 2629746, "months": 2629746,
	"y": 31556952, "year": 31556952, "years": 31556952,
}

// ParseDurationSeconds converts a systemd time span ("5", "500ms", "2min",
// "1h 30min", "infinity") to seconds. ok is false for "infinity" and for
// values that cannot be parsed.
func ParseDurationSeconds(val string) (sec float64, ok bool) {
	val = strings.TrimSpace(val)
	if val == "" || strings.EqualFold(val, "infinity") {
		return 0, false
	}
	// A plain number is seconds.
	if f, err := strconv.ParseFloat(val, 64); err == nil {
		return f, true
	}
	matches := durationPartRegex.FindAllStringSubmatch(val, -1)
	if len(matches) == 0 {
		return 0, false
	}
	total := 0.0
	for _, m := range matches {
		if m[1] == "" {
			continue
		}
		num, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			return 0, false
		}
		mult, exists := durationUnits[m[2]]
		if !exists {
			return 0, false
		}
		total += num * mult
	}
	return total, true
}

// durationSecondsInt converts a systemd duration to whole seconds, rounding
// sub-second values up to 1s (both init systems take integer seconds).
func durationSecondsInt(val string, fallback int) int {
	sec, ok := ParseDurationSeconds(val)
	if !ok {
		return fallback
	}
	if sec <= 0 {
		return 0
	}
	return int(math.Ceil(sec))
}

// restartDelaySeconds returns the RestartSec delay in whole seconds.
func restartDelaySeconds(unit *ParsedServiceUnit) int {
	if unit.RestartSec != "" {
		return durationSecondsInt(unit.RestartSec, 1)
	}
	// systemd default RestartSec is 100ms; 1s is the closest usable value.
	return 1
}

func (u *ParsedServiceUnit) isOneshot() bool { return u.Type == "oneshot" }

func (u *ParsedServiceUnit) wantsRestart() bool {
	switch u.Restart {
	case "always", "on-failure", "on-success", "on-abnormal", "on-abort", "on-watchdog":
		return !u.isOneshot()
	}
	return false
}

// preStartCommands returns ExecCondition + ExecStartPre + (for oneshot,
// every ExecStart except the last, which becomes the main command).
// ExecCondition semantics (skip start on failure) are approximated by
// aborting start_pre, which OpenRC/procd both honour.
func preStartCommands(unit *ParsedServiceUnit) []string {
	var cmds []string
	cmds = append(cmds, unit.ExecCondition...)
	cmds = append(cmds, unit.ExecStartPre...)
	if unit.isOneshot() && len(unit.ExecStartCommands) > 1 {
		cmds = append(cmds, unit.ExecStartCommands[:len(unit.ExecStartCommands)-1]...)
	}
	return cmds
}

// --- stdio helpers --------------------------------------------------------

type stdioTarget struct {
	kind   string // inherit, null, file, syslog, tty, console, journal
	path   string
	append bool
}

func parseStdio(val string) stdioTarget {
	val = strings.TrimSpace(val)
	if val == "" {
		return stdioTarget{kind: "inherit"}
	}
	lower := strings.ToLower(val)
	switch {
	case strings.HasPrefix(lower, "file:"):
		return stdioTarget{kind: "file", append: false, path: val[len("file:"):]}
	case strings.HasPrefix(lower, "append:"):
		return stdioTarget{kind: "file", append: true, path: val[len("append:"):]}
	case strings.HasPrefix(lower, "truncate:"):
		return stdioTarget{kind: "file", append: false, path: val[len("truncate:"):]}
	case lower == "null":
		return stdioTarget{kind: "null"}
	case lower == "syslog" || lower == "kmsg" || strings.HasPrefix(lower, "journal"):
		return stdioTarget{kind: "syslog"}
	case lower == "tty" || lower == "console":
		return stdioTarget{kind: "tty"}
	default:
		// inherit and unknown values: leave to the init system's default.
		return stdioTarget{kind: "inherit"}
	}
}

// --- dependency helpers ---------------------------------------------------

var openRCDepMap = map[string]string{
	"network":         "net",
	"network-online":  "net",
	"local-fs":        "localmount",
	"remote-fs":       "remote",
	"syslog":          "logger",
	"time-sync":       "clock",
	"dbus":            "dbus",
	"sound":           "alsa",
	"printer":         "cupsd",
	"bluetooth":       "bluetooth",
	"nss-user-lookup": "nss",
}

// systemdUnitToOpenRC converts a systemd unit name to an OpenRC service
// name ("network.target" -> "net", "foo.service" -> "foo"). The boolean
// result is false for units with no plausible OpenRC counterpart.
func systemdUnitToOpenRC(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}
	base := name
	for _, suffix := range []string{".service", ".target", ".socket", ".timer", ".mount", ".automount", ".path", ".slice", ".scope"} {
		if strings.HasSuffix(base, suffix) {
			base = strings.TrimSuffix(base, suffix)
			break
		}
	}
	if mapped, ok := openRCDepMap[base]; ok {
		return mapped, true
	}
	// A generic .service dependency: depend on the same-named service if
	// present in the runlevel (ordering only, never a hard need).
	if strings.HasSuffix(name, ".service") && base != "" && base != name {
		return base, true
	}
	return "", false
}

// --- unsupported directive notes ------------------------------------------

var alwaysUnsupported = map[string]string{
	"Service.PrivateTmp":               "no equivalent (namespace sandboxing)",
	"Service.PrivateDevices":           "no equivalent (namespace sandboxing)",
	"Service.ProtectSystem":            "no equivalent (namespace sandboxing)",
	"Service.ProtectHome":              "no equivalent (namespace sandboxing)",
	"Service.ReadOnlyPaths":            "no equivalent (namespace sandboxing)",
	"Service.ReadWritePaths":           "no equivalent (namespace sandboxing)",
	"Service.InaccessiblePaths":        "no equivalent (namespace sandboxing)",
	"Service.DynamicUser":              "no equivalent (dynamic user allocation)",
	"Service.RestrictAddressFamilies":  "no equivalent (namespace sandboxing)",
	"Service.RestrictNamespaces":       "no equivalent (namespace sandboxing)",
	"Service.SystemCallFilter":         "no equivalent (seccomp filtering)",
	"Service.SystemCallArchitectures":  "no equivalent (seccomp filtering)",
	"Service.CapabilityBoundingSet":    "no equivalent (only AmbientCapabilities is mapped)",
	"Service.KillMode":                 "no equivalent (process-group kill semantics)",
	"Service.WatchdogSec":              "no equivalent (watchdog notification)",
	"Service.RestartPreventExitStatus": "no equivalent (restart-on-failure filtering is approximated)",
	"Service.RestartForceExitStatus":   "no equivalent (restart-on-failure filtering is approximated)",
	"Service.SuccessExitStatus":        "no equivalent (exit-status filtering)",
	"Service.LimitCPU":                 "no equivalent",
	"Service.MemoryLimit":              "no equivalent (cgroup limits)",
	"Service.MemoryMax":                "no equivalent (cgroup limits)",
	"Service.TasksMax":                 "no equivalent (cgroup limits)",
	"Service.TimeoutStartSec":          "no equivalent (start timeout)",
	"Unit.OnFailure":                   "no equivalent (failure-triggered units)",
	"Unit.PropagatesStopTo":            "no equivalent",
	"Unit.PropagatesReloadTo":          "no equivalent",
	"Unit.RequiresMountsFor":           "no equivalent",
	"Unit.Conflicts":                   "no equivalent (conflict semantics)",
	"Unit.PartOf":                      "no equivalent",
}

var openRCUnsupported = map[string]string{}

var procdUnsupported = map[string]string{
	"Service.KillSignal":          "procd always stops with SIGTERM",
	"Service.StandardInput":       "no equivalent (stdin redirection)",
	"Service.SupplementaryGroups": "no equivalent (only a single group is supported)",
	"Service.After":               "no equivalent (procd has no ordering between services)",
	"Service.Before":              "no equivalent (procd has no ordering between services)",
	"Service.Wants":               "no equivalent (procd has no inter-service dependencies)",
	"Service.Requires":            "no equivalent (procd has no inter-service dependencies)",
	"Service.BindsTo":             "no equivalent (procd has no inter-service dependencies)",
	"Unit.WantedBy":               "no equivalent (procd enablement is unconditional)",
}

func unsupportedNotes(unit *ParsedServiceUnit, target string) []string {
	extra := openRCUnsupported
	if target == "procd" {
		extra = procdUnsupported
	}
	seen := map[string]bool{}
	var notes []string
	add := func(key string, reason string) {
		if seen[key] {
			return
		}
		seen[key] = true
		notes = append(notes, fmt.Sprintf("# NOTE: systemd directive %s was dropped: %s", key, reason))
	}
	for key := range unit.RawDirectives {
		if reason, ok := alwaysUnsupported[key]; ok {
			add(key, reason)
		} else if reason, ok := extra[key]; ok {
			add(key, reason)
		}
	}
	if unit.Type == "notify" || unit.Type == "dbus" {
		add("Service.Type="+unit.Type, "readiness notification is not supported; the service is started like Type=simple")
	}
	if strings.Contains(unit.ExecStart, "%") {
		add("Service.ExecStart (specifiers)", "systemd specifiers (%n, %i, ...) are not expanded")
	}
	// Sorted for deterministic output.
	sort.Strings(notes)
	return notes
}

// --- OpenRC transpilation -------------------------------------------------

// TranspileToOpenRC generates an OpenRC init script (/etc/init.d/ezapp-<name>)
func TranspileToOpenRC(appName string, unit *ParsedServiceUnit) (string, error) {
	if unit.ExecStart == "" {
		return "", fmt.Errorf("cannot transpile to OpenRC: ExecStart is missing")
	}

	ec := stripExecPrefixes(unit.ExecStart)
	cmdParts := splitCommand(ec.raw)
	if len(cmdParts) == 0 {
		return "", fmt.Errorf("invalid ExecStart directive")
	}

	cmdBin := cmdParts[0]
	cmdArgs := ""
	if len(cmdParts) > 1 {
		var escapedArgs []string
		for _, arg := range cmdParts[1:] {
			escapedArgs = append(escapedArgs, ShellEscape(arg))
		}
		cmdArgs = strings.Join(escapedArgs, " ")
	}

	supervise := unit.wantsRestart()
	pidfile := unit.PIDFile
	if pidfile == "" {
		pidfile = fmt.Sprintf("/run/ezapp-%s.pid", appName)
	}
	background := !unit.isOneshot()

	var sb strings.Builder
	sb.WriteString("#!/sbin/openrc-run\n")
	sb.WriteString(fmt.Sprintf("# Easy42 OpenRC Service: ezapp-%s\n", appName))
	sb.WriteString("# Transpiled from a systemd service unit; see generated comments for dropped directives.\n")
	for _, note := range unsupportedNotes(unit, "openrc") {
		sb.WriteString(note + "\n")
	}
	if unit.Description != "" {
		sb.WriteString(fmt.Sprintf("description=%s\n", ShellEscape(unit.Description)))
	} else {
		sb.WriteString(fmt.Sprintf("description=\"Easy42 managed app %s\"\n", appName))
	}

	sb.WriteString(fmt.Sprintf("command=%s\n", ShellEscape(cmdBin)))
	if cmdArgs != "" {
		sb.WriteString(fmt.Sprintf("command_args=\"%s\"\n", cmdArgs))
		if supervise {
			sb.WriteString(fmt.Sprintf("command_args_foreground=\"%s\"\n", cmdArgs))
		}
	}

	if supervise {
		sb.WriteString("supervisor=\"supervise-daemon\"\n")
		sb.WriteString(fmt.Sprintf("respawn_delay=\"%d\"\n", restartDelaySeconds(unit)))
		if unit.Restart != "always" {
			sb.WriteString("# Restart=on-* approximated as restart on any exit\n")
		}
	}

	userGroup := ""
	if !ec.runAsRoot {
		if unit.User != "" && unit.Group != "" {
			userGroup = fmt.Sprintf("%s:%s", unit.User, unit.Group)
		} else if unit.User != "" {
			userGroup = unit.User
		} else if unit.Group != "" {
			// command_user accepts a bare group only in "user:group" form;
			// without a user, run as root with this group is not expressible.
			sb.WriteString("# NOTE: Group without User has no OpenRC equivalent and was dropped\n")
		}
	}
	if userGroup != "" {
		sb.WriteString(fmt.Sprintf("command_user=%s\n", ShellEscape(userGroup)))
	}

	if unit.WorkingDirectory != "" {
		sb.WriteString(fmt.Sprintf("directory=%s\n", ShellEscape(unit.WorkingDirectory)))
	}
	if unit.UMask != "" {
		sb.WriteString(fmt.Sprintf("umask=%s\n", ShellEscape(unit.UMask)))
	}
	if unit.NoNewPrivileges {
		sb.WriteString("no_new_privs=\"yes\"\n")
	}
	if len(unit.AmbientCapabilities) > 0 {
		sb.WriteString(fmt.Sprintf("capabilities=%s\n", ShellEscape(strings.Join(unit.AmbientCapabilities, " "))))
	}
	if niceVal, err := strconv.Atoi(strings.TrimSpace(unit.Nice)); err == nil && unit.Nice != "" {
		sb.WriteString(fmt.Sprintf("start_stop_daemon_args=\"--nice %d\"\n", niceVal))
	}
	if unit.KillSignal != "" {
		sb.WriteString(fmt.Sprintf("stopsig=%s\n", ShellEscape(unit.KillSignal)))
	}
	if unit.TimeoutStopSec != "" {
		stopSec := durationSecondsInt(unit.TimeoutStopSec, 0)
		if stopSec > 0 {
			signal := "TERM"
			if unit.KillSignal != "" {
				signal = strings.TrimPrefix(strings.ToUpper(unit.KillSignal), "SIG")
			}
			sb.WriteString(fmt.Sprintf("retry=\"%s/%d/KILL/5\"\n", signal, stopSec))
		}
	}

	// StandardInput / StandardOutput / StandardError
	stdin := parseStdio(unit.StandardInput)
	if stdin.kind == "null" {
		sb.WriteString("input_file=\"/dev/null\"\n")
	} else if stdin.kind == "file" {
		sb.WriteString(fmt.Sprintf("input_file=%s\n", ShellEscape(stdin.path)))
	}
	writeStdioOpenRC := func(val string, logVar, loggerVar string) {
		t := parseStdio(val)
		switch t.kind {
		case "null":
			sb.WriteString(fmt.Sprintf("%s=\"/dev/null\"\n", logVar))
		case "file":
			sb.WriteString(fmt.Sprintf("%s=%s\n", logVar, ShellEscape(t.path)))
		case "syslog", "tty":
			sb.WriteString(fmt.Sprintf("%s=\"logger -t ezapp-%s\"\n", loggerVar, appName))
		}
	}
	// Only emit when explicitly set; "inherit" is the OpenRC default.
	if unit.StandardOutput != "" {
		writeStdioOpenRC(unit.StandardOutput, "output_log", "output_logger")
	}
	if unit.StandardError != "" {
		writeStdioOpenRC(unit.StandardError, "error_log", "error_logger")
	}

	if background {
		sb.WriteString("command_background=\"yes\"\n")
		sb.WriteString(fmt.Sprintf("pidfile=%s\n", ShellEscape(pidfile)))
	} else {
		sb.WriteString("command_background=\"no\"\n")
		if unit.RemainAfterExit {
			sb.WriteString("# NOTE: RemainAfterExit=yes has no OpenRC equivalent; the oneshot service is considered stopped once it exits\n")
		}
	}

	// depend()
	sb.WriteString("\ndepend() {\n")
	sb.WriteString("    need net\n")
	sb.WriteString("    use dns logger\n")
	var afterList, useList []string
	for _, dep := range append(append([]string{}, unit.After...), unit.Requires...) {
		if svc, ok := systemdUnitToOpenRC(dep); ok && svc != "net" {
			afterList = append(afterList, svc)
		}
	}
	for _, dep := range append(append([]string{}, unit.Wants...), unit.BindsTo...) {
		if svc, ok := systemdUnitToOpenRC(dep); ok && svc != "net" {
			useList = append(useList, svc)
		}
	}
	afterList = dedupe(afterList)
	useList = dedupe(useList)
	if len(afterList) > 0 {
		sb.WriteString(fmt.Sprintf("    after %s\n", strings.Join(afterList, " ")))
	}
	if len(useList) > 0 {
		sb.WriteString(fmt.Sprintf("    use %s\n", strings.Join(useList, " ")))
	}
	var beforeList []string
	for _, dep := range unit.Before {
		if svc, ok := systemdUnitToOpenRC(dep); ok {
			beforeList = append(beforeList, svc)
		}
	}
	if len(beforeList) > 0 {
		sb.WriteString(fmt.Sprintf("    before %s\n", strings.Join(dedupe(beforeList), " ")))
	}
	sb.WriteString("}\n")

	if len(unit.ExecReload) > 0 {
		sb.WriteString("extra_started_commands=\"reload\"\n")
		sb.WriteString("description_reload=\"Run ExecReload command(s)\"\n")
	}

	// start_pre(): env file, resource limits, condition and pre commands.
	pre := preStartCommands(unit)
	limits := ulimitLines(unit)
	if len(pre) > 0 || len(limits) > 0 {
		sb.WriteString("\nstart_pre() {\n")
		sb.WriteString(fmt.Sprintf("    if [ -f \"/etc/conf.d/ezapp-%s\" ]; then\n", appName))
		sb.WriteString(fmt.Sprintf("        . \"/etc/conf.d/ezapp-%s\"\n", appName))
		sb.WriteString("    fi\n")
		for _, l := range limits {
			sb.WriteString("    " + l + "\n")
		}
		for _, cmd := range pre {
			if line := shellHookLine(cmd, "return"); line != "" {
				sb.WriteString("    " + line + "\n")
			}
		}
		sb.WriteString("}\n")
	} else {
		sb.WriteString("\nstart_pre() {\n")
		sb.WriteString(fmt.Sprintf("    if [ -f \"/etc/conf.d/ezapp-%s\" ]; then\n", appName))
		sb.WriteString(fmt.Sprintf("        . \"/etc/conf.d/ezapp-%s\"\n", appName))
		sb.WriteString("    fi\n")
		sb.WriteString("}\n")
	}

	if len(unit.ExecStartPost) > 0 {
		sb.WriteString("\nstart_post() {\n")
		for _, cmd := range unit.ExecStartPost {
			if line := shellHookLine(cmd, "return"); line != "" {
				sb.WriteString("    " + line + "\n")
			}
		}
		sb.WriteString("}\n")
	}

	if len(unit.ExecStop) > 0 {
		sb.WriteString("\nstop_pre() {\n")
		for _, cmd := range unit.ExecStop {
			if line := shellHookLine(cmd, "ignore"); line != "" {
				sb.WriteString("    " + line + "\n")
			}
		}
		sb.WriteString("}\n")
	}

	if len(unit.ExecStopPost) > 0 {
		sb.WriteString("\nstop_post() {\n")
		for _, cmd := range unit.ExecStopPost {
			if line := shellHookLine(cmd, "ignore"); line != "" {
				sb.WriteString("    " + line + "\n")
			}
		}
		sb.WriteString("}\n")
	}

	if len(unit.ExecReload) > 0 {
		sb.WriteString("\nreload() {\n")
		for _, cmd := range unit.ExecReload {
			if line := shellHookLine(cmd, "return"); line != "" {
				sb.WriteString("    " + line + "\n")
			}
		}
		sb.WriteString("}\n")
	}

	return sb.String(), nil
}

// ulimitLines converts LimitNOFILE/LimitNPROC/LimitCORE to shell ulimit lines.
func ulimitLines(unit *ParsedServiceUnit) []string {
	var lines []string
	for _, l := range []struct{ val, flag string }{
		{unit.LimitNOFILE, "-n"},
		{unit.LimitNPROC, "-u"},
		{unit.LimitCORE, "-c"},
	} {
		if l.val == "" {
			continue
		}
		v := strings.TrimSpace(l.val)
		if strings.EqualFold(v, "infinity") {
			v = "unlimited"
		}
		if !isSafeUlimitValue(v) {
			continue
		}
		lines = append(lines, fmt.Sprintf("ulimit %s %s", l.flag, v))
	}
	return lines
}

func isSafeUlimitValue(v string) bool {
	if v == "unlimited" {
		return true
	}
	_, err := strconv.ParseUint(v, 10, 64)
	return err == nil
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// GenerateOpenRCConfD generates /etc/conf.d/ezapp-<name> with explicit export directives
func GenerateOpenRCConfD(appName string, envVars map[string]string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Easy42 environment configuration for ezapp-%s\n", appName))
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sb.WriteString(fmt.Sprintf("export %s=%s\n", k, ShellEscape(envVars[k])))
	}
	return sb.String()
}

// --- procd transpilation --------------------------------------------------

// TranspileToProcd generates an OpenWrt procd init script (/etc/init.d/ezapp-<name>)
func TranspileToProcd(appName string, unit *ParsedServiceUnit, envVars map[string]string) (string, error) {
	if unit.ExecStart == "" {
		return "", fmt.Errorf("cannot transpile to Procd: ExecStart is missing")
	}

	ec := stripExecPrefixes(unit.ExecStart)
	cmdParts := splitCommand(ec.raw)
	if len(cmdParts) == 0 {
		return "", fmt.Errorf("invalid ExecStart directive")
	}

	var sb strings.Builder
	sb.WriteString("#!/bin/sh /etc/rc.common\n")
	sb.WriteString(fmt.Sprintf("# Easy42 OpenWrt Procd Service: ezapp-%s\n", appName))
	sb.WriteString("# Transpiled from a systemd service unit; see generated comments for dropped directives.\n")
	for _, note := range unsupportedNotes(unit, "procd") {
		sb.WriteString(note + "\n")
	}
	sb.WriteString("USE_PROCD=1\n")
	sb.WriteString("START=95\n")
	sb.WriteString("STOP=10\n\n")

	sb.WriteString("start_service() {\n")

	// ExecCondition / ExecStartPre run synchronously before the instance
	// is opened, which matches systemd ordering.
	for _, cmd := range preStartCommands(unit) {
		if line := shellHookLine(cmd, "return"); line != "" {
			sb.WriteString("    " + line + "\n")
		}
	}

	sb.WriteString(fmt.Sprintf("    procd_open_instance \"ezapp-%s\"\n", appName))

	// procd_set_param command bin arg1 arg2
	// procd has no cwd/umask/stdio-file parameters, so when any of those
	// are needed the command runs through /bin/sh -c with an exec, which
	// preserves signal handling and the exit status.
	commandParts := cmdParts
	if wrapper, ok := procdShellWrapper(unit, cmdParts); ok {
		commandParts = wrapper
	}
	var escapedCommand []string
	for _, p := range commandParts {
		escapedCommand = append(escapedCommand, ShellEscape(p))
	}
	sb.WriteString(fmt.Sprintf("    procd_set_param command %s\n", strings.Join(escapedCommand, " ")))

	// respawn policy: procd respawn is "<fail_threshold> <restart_timeout> <max_fail>"
	if unit.wantsRestart() {
		sb.WriteString(fmt.Sprintf("    procd_set_param respawn 3600 %d 5\n", restartDelaySeconds(unit)))
	}

	if !ec.runAsRoot && unit.User != "" {
		sb.WriteString(fmt.Sprintf("    procd_set_param user %s\n", ShellEscape(unit.User)))
	}
	if !ec.runAsRoot && unit.Group != "" {
		sb.WriteString(fmt.Sprintf("    procd_set_param group %s\n", ShellEscape(unit.Group)))
	}
	if unit.PIDFile != "" {
		sb.WriteString(fmt.Sprintf("    procd_set_param pidfile %s\n", ShellEscape(unit.PIDFile)))
	}
	if niceVal, err := strconv.Atoi(strings.TrimSpace(unit.Nice)); err == nil && unit.Nice != "" {
		sb.WriteString(fmt.Sprintf("    procd_set_param nice %d\n", niceVal))
	}
	if unit.TimeoutStopSec != "" {
		if stopSec := durationSecondsInt(unit.TimeoutStopSec, 0); stopSec > 0 {
			sb.WriteString(fmt.Sprintf("    procd_set_param term_timeout %d\n", stopSec))
		}
	}
	if unit.NoNewPrivileges {
		sb.WriteString("    procd_set_param no_new_privs 1\n")
	}
	if len(unit.AmbientCapabilities) > 0 {
		sb.WriteString(fmt.Sprintf("    procd_set_param capabilities %s\n", ShellEscape(strings.Join(unit.AmbientCapabilities, " "))))
	}
	for _, l := range []struct{ val, key string }{
		{unit.LimitNOFILE, "nofile"},
		{unit.LimitNPROC, "nproc"},
		{unit.LimitCORE, "core"},
	} {
		if l.val == "" {
			continue
		}
		v := strings.TrimSpace(l.val)
		if strings.EqualFold(v, "infinity") {
			v = "unlimited"
		}
		if !isSafeUlimitValue(v) {
			continue
		}
		sb.WriteString(fmt.Sprintf("    procd_set_param limits %s=\"%s\"\n", l.key, v))
	}

	// Stdout/stderr forwarded to syslog unless redirected to a file by the
	// shell wrapper.
	stdout := parseStdio(unit.StandardOutput)
	stderr := parseStdio(unit.StandardError)
	if stdout.kind == "syslog" || stdout.kind == "inherit" || stdout.kind == "tty" {
		sb.WriteString("    procd_set_param stdout 1\n")
	}
	if stderr.kind == "syslog" || stderr.kind == "inherit" || stderr.kind == "tty" {
		sb.WriteString("    procd_set_param stderr 1\n")
	}

	// env vars
	if len(envVars) > 0 {
		keys := make([]string, 0, len(envVars))
		for k := range envVars {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var envParts []string
		for _, k := range keys {
			envParts = append(envParts, fmt.Sprintf("%s=%s", k, ShellEscape(envVars[k])))
		}
		sb.WriteString(fmt.Sprintf("    procd_set_param env %s\n", strings.Join(envParts, " ")))
	}

	sb.WriteString("    procd_close_instance\n")

	if len(unit.ExecStartPost) > 0 {
		sb.WriteString("    # NOTE: procd starts the instance asynchronously, so ExecStartPost may run before the main process is ready\n")
		for _, cmd := range unit.ExecStartPost {
			if line := shellHookLine(cmd, "return"); line != "" {
				sb.WriteString("    " + line + "\n")
			}
		}
	}
	sb.WriteString("}\n")

	if len(unit.ExecStop) > 0 || len(unit.ExecStopPost) > 0 {
		sb.WriteString("\nstop_service() {\n")
		for _, cmd := range unit.ExecStop {
			if line := shellHookLine(cmd, "ignore"); line != "" {
				sb.WriteString("    " + line + "\n")
			}
		}
		for _, cmd := range unit.ExecStopPost {
			if line := shellHookLine(cmd, "ignore"); line != "" {
				sb.WriteString("    " + line + "\n")
			}
		}
		sb.WriteString("}\n")
	}

	if len(unit.ExecReload) > 0 {
		sb.WriteString("\nreload_service() {\n")
		for _, cmd := range unit.ExecReload {
			if line := shellHookLine(cmd, "return"); line != "" {
				sb.WriteString("    " + line + "\n")
			}
		}
		sb.WriteString("}\n")
	}

	return sb.String(), nil
}

// procdShellWrapper builds a /bin/sh -c command when the unit needs shell
// features procd cannot express natively: working directory, umask, or
// stdout/stderr redirection to a file.
func procdShellWrapper(unit *ParsedServiceUnit, cmdParts []string) ([]string, bool) {
	stdout := parseStdio(unit.StandardOutput)
	stderr := parseStdio(unit.StandardError)
	needFile := stdout.kind == "file" || stdout.kind == "null" ||
		stderr.kind == "file" || stderr.kind == "null"
	if unit.WorkingDirectory == "" && unit.UMask == "" && !needFile {
		return nil, false
	}

	script := ""
	if unit.WorkingDirectory != "" {
		script += fmt.Sprintf("cd %s", ShellEscape(unit.WorkingDirectory))
	}
	if unit.UMask != "" {
		if script != "" {
			script += " && "
		}
		script += fmt.Sprintf("umask %s", unit.UMask)
	}
	if script != "" {
		script += " && "
	}
	script += "exec"
	for _, p := range cmdParts {
		script += " " + ShellEscape(p)
	}
	if stdout.kind == "file" {
		op := ">"
		if stdout.append {
			op = ">>"
		}
		script += fmt.Sprintf(" %s %s", op, ShellEscape(stdout.path))
	} else if stdout.kind == "null" {
		script += " >/dev/null"
	}
	if stderr.kind == "file" {
		op := "2>"
		if stderr.append {
			op = "2>>"
		}
		script += fmt.Sprintf(" %s %s", op, ShellEscape(stderr.path))
	} else if stderr.kind == "null" {
		script += " 2>/dev/null"
	}
	return []string{"/bin/sh", "-c", script}, true
}

// splitCommand parses command line taking basic quotes into account
func splitCommand(cmd string) []string {
	var res []string
	var cur strings.Builder
	inQuotes := false
	quoteChar := byte(0)

	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		if inQuotes {
			if c == quoteChar {
				inQuotes = false
			} else {
				cur.WriteByte(c)
			}
		} else {
			if c == '"' || c == '\'' {
				inQuotes = true
				quoteChar = c
			} else if c == ' ' || c == '\t' {
				if cur.Len() > 0 {
					res = append(res, cur.String())
					cur.Reset()
				}
			} else {
				cur.WriteByte(c)
			}
		}
	}
	if cur.Len() > 0 {
		res = append(res, cur.String())
	}
	return res
}

// EnsureSystemdEnvironmentFile ensures that the systemd service content contains
// EnvironmentFile=/etc/systemd/system/ezapp-<appName>.env under [Service].
func EnsureSystemdEnvironmentFile(appName string, content string) string {
	appName = strings.TrimSpace(appName)
	if appName == "" {
		appName = "sample"
	}
	expectedPath := fmt.Sprintf("/etc/systemd/system/ezapp-%s.env", appName)

	if strings.Contains(content, expectedPath) {
		return content
	}

	samplePath := "/etc/systemd/system/ezapp-sample.env"
	if strings.Contains(content, samplePath) {
		return strings.Replace(content, samplePath, expectedPath, 1)
	}

	expectedDirective := fmt.Sprintf("EnvironmentFile=%s", expectedPath)

	lines := strings.Split(content, "\n")
	serviceIdx := -1
	for i, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "[Service]") {
			serviceIdx = i
			break
		}
	}

	if serviceIdx != -1 {
		newLines := make([]string, 0, len(lines)+1)
		newLines = append(newLines, lines[:serviceIdx+1]...)
		newLines = append(newLines, expectedDirective)
		newLines = append(newLines, lines[serviceIdx+1:]...)
		return strings.Join(newLines, "\n")
	}

	trimmedContent := strings.TrimRight(content, "\r\n")
	if trimmedContent == "" {
		return fmt.Sprintf("[Service]\n%s\n", expectedDirective)
	}
	return fmt.Sprintf("%s\n\n[Service]\n%s\n", trimmedContent, expectedDirective)
}
