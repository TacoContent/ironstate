//go:build windows

package handlers

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/TacoContent/ironstate/internal/engine"
)

// scheduledTaskHandler ports Handlers/ScheduledTask.psm1: registers/
// updates/removes a Windows Task Scheduler task.
//
// Deviation from Handlers/ScheduledTask.psm1 (documented, not silent),
// chosen deliberately to avoid adding a second PowerShell dependency
// beyond 'shell.host: pwsh' (docs/plans/go-rewrite.md §11 - "the sole
// remaining runtime dependency"): this is implemented by generating a
// Task Scheduler XML definition and shelling out to the built-in
// 'schtasks.exe' (part of Windows itself, not PowerShell), instead of the
// ScheduledTasks PowerShell module/CIM. A real consequence of that
// choice: 'schtasks.exe' has no equivalent of Get-ScheduledTask's rich
// object model to diff field-by-field, so **idempotency is reduced to
// existence + enabled-state checking only** - unlike the original, a
// registered task whose actions/triggers/principal/settings have drifted
// from the declared YAML is NOT detected or auto-corrected under
// 'state: present'. Use 'state: latest' to force re-registration
// (Task Scheduler's own Register-equivalent, '/Create /F', always
// overwrites) whenever a task's declared shape may have changed.
type scheduledTaskHandler struct{}

func resolveTaskFolderPath(p string) string {
	if p == "" {
		return `\`
	}
	p = strings.TrimSpace(p)
	if !strings.HasPrefix(p, `\`) {
		p = `\` + p
	}
	if !strings.HasSuffix(p, `\`) {
		p += `\`
	}
	return p
}

func taskFullName(item map[string]any) string {
	name := getString(item, "name")
	folder := resolveTaskFolderPath(getString(item, "path"))
	return folder + name
}

// --- duration helpers -------------------------------------------------

func parseTaskDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	if strings.HasPrefix(s, "P") {
		return parseISO8601Duration(s)
	}
	if strings.Contains(s, ":") {
		return parseDotNetTimeSpan(s)
	}
	return time.ParseDuration(s)
}

func parseISO8601Duration(s string) (time.Duration, error) {
	orig := s
	s = strings.TrimPrefix(s, "P")
	var days, hours, minutes int
	var seconds float64
	datePart, timePart, hasTime := strings.Cut(s, "T")
	if d := datePart; d != "" {
		if n, err := parseISONumberUnit(d, "D"); err == nil {
			days = n
		} else {
			return 0, fmt.Errorf("unsupported ISO 8601 duration %q", orig)
		}
	}
	if hasTime {
		rest := timePart
		if idx := strings.IndexByte(rest, 'H'); idx >= 0 {
			hours, _ = strconv.Atoi(rest[:idx])
			rest = rest[idx+1:]
		}
		if idx := strings.IndexByte(rest, 'M'); idx >= 0 {
			minutes, _ = strconv.Atoi(rest[:idx])
			rest = rest[idx+1:]
		}
		if idx := strings.IndexByte(rest, 'S'); idx >= 0 {
			seconds, _ = strconv.ParseFloat(rest[:idx], 64)
		}
	}
	total := time.Duration(days)*24*time.Hour + time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute + time.Duration(seconds*float64(time.Second))
	return total, nil
}

func parseISONumberUnit(s, unit string) (int, error) {
	if !strings.HasSuffix(s, unit) {
		return 0, fmt.Errorf("expected suffix %q", unit)
	}
	return strconv.Atoi(strings.TrimSuffix(s, unit))
}

// parseDotNetTimeSpan accepts '[d.]hh:mm:ss[.fffffff]'.
func parseDotNetTimeSpan(s string) (time.Duration, error) {
	days := 0
	rest := s
	if idx := strings.IndexByte(s, '.'); idx >= 0 && idx < strings.IndexByte(s, ':') {
		var err error
		days, err = strconv.Atoi(s[:idx])
		if err != nil {
			return 0, err
		}
		rest = s[idx+1:]
	}
	parts := strings.Split(rest, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid TimeSpan %q", s)
	}
	hours, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}
	minutes, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}
	seconds, err := strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return 0, err
	}
	return time.Duration(days)*24*time.Hour + time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute + time.Duration(seconds*float64(time.Second)), nil
}

func formatISO8601Duration(d time.Duration) string {
	if d <= 0 {
		return "PT0S"
	}
	days := int64(d / (24 * time.Hour))
	rem := d % (24 * time.Hour)
	hours := int64(rem / time.Hour)
	rem %= time.Hour
	minutes := int64(rem / time.Minute)
	rem %= time.Minute
	seconds := rem.Seconds()

	var sb strings.Builder
	sb.WriteString("P")
	if days > 0 {
		fmt.Fprintf(&sb, "%dD", days)
	}
	if hours > 0 || minutes > 0 || seconds > 0 {
		sb.WriteString("T")
		if hours > 0 {
			fmt.Fprintf(&sb, "%dH", hours)
		}
		if minutes > 0 {
			fmt.Fprintf(&sb, "%dM", minutes)
		}
		if seconds > 0 {
			if seconds == float64(int64(seconds)) {
				fmt.Fprintf(&sb, "%dS", int64(seconds))
			} else {
				fmt.Fprintf(&sb, "%gS", seconds)
			}
		}
	}
	return sb.String()
}

func durationField(item map[string]any, key string) string {
	s := getString(item, key)
	if s == "" {
		return ""
	}
	d, err := parseTaskDuration(s)
	if err != nil {
		return ""
	}
	return formatISO8601Duration(d)
}

// --- XML generation ----------------------------------------------------

func xmlEscapeText(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}

func buildTaskTriggerXML(trigger map[string]any) (string, error) {
	triggerType := strings.ToLower(getString(trigger, "type"))
	randomDelay := durationField(trigger, "random_delay")

	switch triggerType {
	case "logon":
		var sb strings.Builder
		sb.WriteString("<LogonTrigger>")
		if userID := getString(trigger, "user_id"); userID != "" {
			fmt.Fprintf(&sb, "<UserId>%s</UserId>", xmlEscapeText(userID))
		}
		if delay := durationField(trigger, "delay"); delay != "" {
			fmt.Fprintf(&sb, "<Delay>%s</Delay>", delay)
		}
		if randomDelay != "" {
			fmt.Fprintf(&sb, "<RandomDelay>%s</RandomDelay>", randomDelay)
		}
		sb.WriteString("</LogonTrigger>")
		return sb.String(), nil
	case "startup":
		var sb strings.Builder
		sb.WriteString("<BootTrigger>")
		if delay := durationField(trigger, "delay"); delay != "" {
			fmt.Fprintf(&sb, "<Delay>%s</Delay>", delay)
		}
		if randomDelay != "" {
			fmt.Fprintf(&sb, "<RandomDelay>%s</RandomDelay>", randomDelay)
		}
		sb.WriteString("</BootTrigger>")
		return sb.String(), nil
	case "once":
		at, err := parseTaskDateTime(getString(trigger, "at"))
		if err != nil {
			return "", err
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "<TimeTrigger><StartBoundary>%s</StartBoundary>", at.Format("2006-01-02T15:04:05"))
		if randomDelay != "" {
			fmt.Fprintf(&sb, "<RandomDelay>%s</RandomDelay>", randomDelay)
		}
		writeTaskRepetition(&sb, trigger)
		sb.WriteString("</TimeTrigger>")
		return sb.String(), nil
	case "daily":
		at, err := parseTaskDateTime(getString(trigger, "at"))
		if err != nil {
			return "", err
		}
		daysInterval := 1
		if v, ok := trigger["days_interval"]; ok {
			daysInterval = int(toFloat(v))
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "<CalendarTrigger><StartBoundary>%s</StartBoundary>", at.Format("2006-01-02T15:04:05"))
		if randomDelay != "" {
			fmt.Fprintf(&sb, "<RandomDelay>%s</RandomDelay>", randomDelay)
		}
		writeTaskRepetition(&sb, trigger)
		fmt.Fprintf(&sb, "<ScheduleByDay><DaysInterval>%d</DaysInterval></ScheduleByDay></CalendarTrigger>", daysInterval)
		return sb.String(), nil
	case "weekly":
		at, err := parseTaskDateTime(getString(trigger, "at"))
		if err != nil {
			return "", err
		}
		days := asList(trigger["days_of_week"])
		if len(days) == 0 {
			return "", fmt.Errorf("scheduled_task weekly trigger requires 'days_of_week'")
		}
		weeksInterval := 1
		if v, ok := trigger["weeks_interval"]; ok {
			weeksInterval = int(toFloat(v))
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "<CalendarTrigger><StartBoundary>%s</StartBoundary>", at.Format("2006-01-02T15:04:05"))
		if randomDelay != "" {
			fmt.Fprintf(&sb, "<RandomDelay>%s</RandomDelay>", randomDelay)
		}
		writeTaskRepetition(&sb, trigger)
		sb.WriteString("<ScheduleByWeek><DaysOfWeek>")
		for _, raw := range days {
			if name, ok := raw.(string); ok {
				fmt.Fprintf(&sb, "<%s/>", strings.Title(strings.ToLower(name))) //nolint:staticcheck // simple ASCII day-name casing, not locale text
			}
		}
		fmt.Fprintf(&sb, "</DaysOfWeek><WeeksInterval>%d</WeeksInterval></ScheduleByWeek></CalendarTrigger>", weeksInterval)
		return sb.String(), nil
	default:
		return "", fmt.Errorf("unknown scheduled_task trigger type '%s' (expected one of: logon, startup, once, daily, weekly)", triggerType)
	}
}

func writeTaskRepetition(sb *strings.Builder, trigger map[string]any) {
	interval := durationField(trigger, "repetition_interval")
	if interval == "" {
		return
	}
	duration := durationField(trigger, "repetition_duration")
	sb.WriteString("<Repetition>")
	fmt.Fprintf(sb, "<Interval>%s</Interval>", interval)
	if duration != "" {
		fmt.Fprintf(sb, "<Duration>%s</Duration>", duration)
	}
	sb.WriteString("<StopAtDurationEnd>false</StopAtDurationEnd></Repetition>")
}

func parseTaskDateTime(s string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02T15:04:05", time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized scheduled_task 'at' value %q", s)
}

func toFloat(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	}
	return 0
}

var taskSettingsFieldMap = map[string]string{
	"disallow_start_if_on_batteries": "DisallowStartIfOnBatteries",
	"start_when_available":           "StartWhenAvailable",
	"hidden":                         "Hidden",
	"wake_to_run":                    "WakeToRun",
	"allow_hard_terminate":           "AllowHardTerminate",
	"run_only_if_network_available":  "RunOnlyIfNetworkAvailable",
	"run_only_if_idle":               "RunOnlyIfIdle",
}

func buildTaskSettingsXML(item map[string]any) string {
	var sb strings.Builder
	sb.WriteString("<Settings>")
	fmt.Fprintf(&sb, "<Enabled>%v</Enabled>", getBool(item, "enabled", true))
	sb.WriteString("<Hidden>false</Hidden>")

	settings := getMap(item, "settings")
	for key, xmlName := range taskSettingsFieldMap {
		if settings == nil {
			continue
		}
		if v, ok := settings[key]; ok {
			fmt.Fprintf(&sb, "<%s>%v</%s>", xmlName, asBool(v), xmlName)
		}
	}
	if settings != nil {
		if mi, ok := settings["multiple_instances"].(string); ok && mi != "" {
			fmt.Fprintf(&sb, "<MultipleInstancesPolicy>%s</MultipleInstancesPolicy>", xmlEscapeText(mi))
		}
		if d := durationField(settings, "execution_time_limit"); d != "" {
			fmt.Fprintf(&sb, "<ExecutionTimeLimit>%s</ExecutionTimeLimit>", d)
		}
		if d := durationField(settings, "restart_interval"); d != "" {
			restartCount := 1
			if v, ok := settings["restart_count"]; ok {
				restartCount = int(toFloat(v))
			}
			fmt.Fprintf(&sb, "<RestartOnFailure><Interval>%s</Interval><Count>%d</Count></RestartOnFailure>", d, restartCount)
		}
		if d := durationField(settings, "delete_expired_task_after"); d != "" {
			fmt.Fprintf(&sb, "<DeleteExpiredTaskAfter>%s</DeleteExpiredTaskAfter>", d)
		}
	}
	sb.WriteString("</Settings>")
	return sb.String()
}

func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}

var taskRunLevelMap = map[string]string{"Highest": "HighestAvailable", "Limited": "LeastPrivilege"}

func buildTaskPrincipalXML(item map[string]any) string {
	principal := getMap(item, "principal")
	var sb strings.Builder
	sb.WriteString(`<Principals><Principal id="Author">`)
	if principal != nil {
		if userID := getString(principal, "user_id"); userID != "" {
			fmt.Fprintf(&sb, "<UserId>%s</UserId>", xmlEscapeText(userID))
		}
		if groupID := getString(principal, "group_id"); groupID != "" {
			fmt.Fprintf(&sb, "<GroupId>%s</GroupId>", xmlEscapeText(groupID))
		}
		if logonType := getString(principal, "logon_type"); logonType != "" {
			fmt.Fprintf(&sb, "<LogonType>%s</LogonType>", xmlEscapeText(logonType))
		}
		runLevel := getStringOr(principal, "run_level", "Limited")
		if mapped, ok := taskRunLevelMap[runLevel]; ok {
			runLevel = mapped
		}
		fmt.Fprintf(&sb, "<RunLevel>%s</RunLevel>", runLevel)
	} else {
		sb.WriteString("<RunLevel>LeastPrivilege</RunLevel>")
	}
	sb.WriteString("</Principal></Principals>")
	return sb.String()
}

func buildTaskActionsXML(item map[string]any) (string, error) {
	actions := asList(item["actions"])
	if len(actions) == 0 {
		return "", fmt.Errorf("scheduled_task '%s' has no 'actions'", getString(item, "name"))
	}
	var sb strings.Builder
	sb.WriteString(`<Actions Context="Author">`)
	for _, raw := range actions {
		a, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		sb.WriteString("<Exec>")
		fmt.Fprintf(&sb, "<Command>%s</Command>", xmlEscapeText(resolvePath(getString(a, "execute"))))
		if args := getString(a, "arguments"); args != "" {
			fmt.Fprintf(&sb, "<Arguments>%s</Arguments>", xmlEscapeText(args))
		}
		if wd := getString(a, "working_directory"); wd != "" {
			fmt.Fprintf(&sb, "<WorkingDirectory>%s</WorkingDirectory>", xmlEscapeText(wd))
		}
		sb.WriteString("</Exec>")
	}
	sb.WriteString("</Actions>")
	return sb.String(), nil
}

func buildTaskDefinitionXML(item map[string]any) (string, error) {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-16"?>`)
	sb.WriteString(`<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">`)
	if desc := getString(item, "description"); desc != "" {
		fmt.Fprintf(&sb, "<RegistrationInfo><Description>%s</Description></RegistrationInfo>", xmlEscapeText(desc))
	}

	triggers := asList(item["triggers"])
	if len(triggers) > 0 {
		sb.WriteString("<Triggers>")
		for _, raw := range triggers {
			t, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			x, err := buildTaskTriggerXML(t)
			if err != nil {
				return "", err
			}
			sb.WriteString(x)
		}
		sb.WriteString("</Triggers>")
	}

	sb.WriteString(buildTaskPrincipalXML(item))
	sb.WriteString(buildTaskSettingsXML(item))

	actionsXML, err := buildTaskActionsXML(item)
	if err != nil {
		return "", err
	}
	sb.WriteString(actionsXML)
	sb.WriteString("</Task>")
	return sb.String(), nil
}

// writeUTF16File writes content as UTF-16LE with a BOM - the encoding
// schtasks.exe /Create /XML expects.
func writeUTF16File(path, content string) error {
	runes := utf16.Encode([]rune(content))
	buf := make([]byte, 2+len(runes)*2)
	buf[0], buf[1] = 0xFF, 0xFE // BOM (little-endian)
	for i, r := range runes {
		buf[2+i*2] = byte(r)
		buf[2+i*2+1] = byte(r >> 8)
	}
	return os.WriteFile(path, buf, 0o600)
}

func (scheduledTaskHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	full := taskFullName(item)
	result, err := runner.Run("schtasks.exe", []string{"/Query", "/TN", full})
	exists := err == nil && result.RC == 0

	if itemState(item) == "absent" {
		return exists, nil
	}
	if !exists {
		return false, nil
	}
	// Reduced idempotency (see the type doc comment): only existence is
	// checked for 'present' - drift in actions/triggers/principal/settings
	// is not detected. 'enabled' is checked since /Query's CSV output
	// reports it cheaply and it's the one field worth not re-registering
	// over on every run.
	desiredEnabled := getBool(item, "enabled", true)
	csv, err := runner.Run("schtasks.exe", []string{"/Query", "/TN", full, "/FO", "CSV", "/NH"})
	if err != nil || csv.RC != 0 {
		return true, nil
	}
	fields := strings.Split(strings.TrimSpace(csv.Stdout), ",")
	if len(fields) >= 4 {
		status := strings.Trim(fields[3], `"`)
		currentlyEnabled := !strings.EqualFold(status, "Disabled")
		return currentlyEnabled == desiredEnabled, nil
	}
	return true, nil
}

func (scheduledTaskHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	full := taskFullName(item)
	if action == engine.ActionUninstall {
		return "unregister scheduled task '" + full + "'", nil
	}
	actionCount := len(asList(item["actions"]))
	triggerCount := len(asList(item["triggers"]))
	return fmt.Sprintf("register scheduled task '%s' (%d action(s), %d trigger(s))", full, actionCount, triggerCount), nil
}

func (scheduledTaskHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	full := taskFullName(item)
	xmlDoc, err := buildTaskDefinitionXML(item)
	if err != nil {
		return engine.ExecResult{}, err
	}

	tempFile, err := os.CreateTemp("", "ironstate-task-*.xml")
	if err != nil {
		return engine.ExecResult{}, err
	}
	tempPath := tempFile.Name()
	tempFile.Close()
	defer os.Remove(tempPath)
	if err := writeUTF16File(tempPath, xmlDoc); err != nil {
		return engine.ExecResult{}, err
	}

	args := []string{"/Create", "/TN", full, "/XML", tempPath, "/F"}
	if principal := getMap(item, "principal"); principal != nil && getString(principal, "logon_type") == "Password" {
		userID := getString(principal, "user_id")
		if userID == "" {
			return engine.ExecResult{}, fmt.Errorf("scheduled_task '%s' principal.logon_type is 'Password' but no 'user_id' was given", getString(item, "name"))
		}
		passwordEnvName := getString(principal, "password_env")
		password := os.Getenv(passwordEnvName)
		if passwordEnvName == "" || password == "" {
			return engine.ExecResult{}, fmt.Errorf("scheduled_task '%s' principal.logon_type is 'Password' but 'password_env' (%s) is not set", getString(item, "name"), passwordEnvName)
		}
		args = append(args, "/RU", userID, "/RP", password)
	}

	result := runExternalCommand("schtasks.exe", args)
	if result.RC != 0 {
		engine.Warn("schtasks.exe /Create %s exited with code %d", full, result.RC)
	}
	return result, nil
}

func (scheduledTaskHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	full := taskFullName(item)
	result := runExternalCommand("schtasks.exe", []string{"/Delete", "/TN", full, "/F"})
	if result.RC != 0 {
		engine.Warn("schtasks.exe /Delete %s exited with code %d", full, result.RC)
	}
	return result, nil
}
