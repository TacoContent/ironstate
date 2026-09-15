package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/TacoContent/ironstate/internal/engine"
)

// sshHostBlockHandler ports Handlers/SshHostBlock.psm1: renders one or
// more ssh_config "Host" blocks from structured data and writes them into
// a marker-delimited block, reusing blockinfile's marker/insert/backup
// machinery.
type sshHostBlockHandler struct{}

func (sshHostBlockHandler) Emoji() string           { return "🔐" }
func (sshHostBlockHandler) RequiredTools() []string { return []string{} }

var sshHostNameKeys = map[string]bool{"host_name": true, "hostname": true}

// convertSshDirectiveKeyToPascalCase ports
// Convert-SshDirectiveKeyToPascalCase: 'host_name'/'hostName'/'HostName'
// all become 'HostName'.
func convertSshDirectiveKeyToPascalCase(name string) string {
	var withUnderscores strings.Builder
	runes := []rune(name)
	for i, r := range runes {
		withUnderscores.WriteRune(r)
		if i+1 < len(runes) {
			cur := runes[i]
			next := runes[i+1]
			curIsLowerOrDigit := (cur >= 'a' && cur <= 'z') || (cur >= '0' && cur <= '9')
			nextIsUpper := next >= 'A' && next <= 'Z'
			if curIsLowerOrDigit && nextIsUpper {
				withUnderscores.WriteRune('_')
			}
		}
	}
	segments := regexp.MustCompile(`[_\s-]+`).Split(withUnderscores.String(), -1)
	var sb strings.Builder
	for _, seg := range segments {
		if seg == "" {
			continue
		}
		sb.WriteString(strings.ToUpper(seg[:1]) + strings.ToLower(seg[1:]))
	}
	return sb.String()
}

func sshDirectiveValueString(v any) string {
	if b, ok := v.(bool); ok {
		if b {
			return "yes"
		}
		return "no"
	}
	return fmt.Sprintf("%v", v)
}

func sshCommentFieldString(v any) string {
	if v == nil {
		return ""
	}
	if list, ok := v.([]any); ok {
		parts := make([]string, len(list))
		for i, item := range list {
			parts[i] = fmt.Sprintf("%v", item)
		}
		return strings.Join(parts, ", ")
	}
	return sshDirectiveValueString(v)
}

var sshCommentTemplateKeyPattern = regexp.MustCompile(`\{(\w+)\}`)

func sshCommentTemplateKeys(tmpl string) []string {
	if tmpl == "" {
		return nil
	}
	matches := sshCommentTemplateKeyPattern.FindAllStringSubmatch(tmpl, -1)
	keys := make([]string, len(matches))
	for i, m := range matches {
		keys[i] = m[1]
	}
	return keys
}

func expandSshCommentTemplate(tmpl string, entry map[string]any) string {
	result := tmpl
	for _, key := range sshCommentTemplateKeys(tmpl) {
		value := ""
		if v, ok := entry[key]; ok {
			value = sshCommentFieldString(v)
		}
		result = strings.ReplaceAll(result, "{"+key+"}", value)
	}
	return result
}

func mergeSshHostEntry(entry, defaults map[string]any) map[string]any {
	merged := map[string]any{}
	for k, v := range defaults {
		merged[k] = v
	}
	for k, v := range entry {
		merged[k] = v
	}
	return merged
}

func getSshHostBlockEntryLines(entry map[string]any, commentTemplate string) ([]string, error) {
	host, _ := entry["host"].(string)
	if host == "" {
		return nil, fmt.Errorf("'ssh_host_block' requires a 'host' key on every host entry")
	}

	reserved := map[string]bool{"host": true, "comment": true}
	for _, k := range sshCommentTemplateKeys(commentTemplate) {
		reserved[k] = true
	}
	hasOwnHostName := false
	for k := range entry {
		if sshHostNameKeys[strings.ToLower(k)] {
			hasOwnHostName = true
			break
		}
	}

	var lines []string
	var comment string
	if c, ok := entry["comment"].(string); ok && c != "" {
		comment = c
	} else if commentTemplate != "" {
		comment = expandSshCommentTemplate(commentTemplate, entry)
	}
	if comment != "" {
		lines = append(lines, "# "+comment)
	}

	lines = append(lines, "Host "+host)
	if !hasOwnHostName {
		lines = append(lines, "  HostName "+host)
	}

	keys := make([]string, 0, len(entry))
	for k := range entry {
		keys = append(keys, k)
	}
	// Sorted for deterministic output - Go's map has no key order to
	// preserve, unlike the original's [ordered] hashtable; only cosmetic
	// line order changes, not ssh_config semantics.
	sort.Strings(keys)
	for _, key := range keys {
		if reserved[key] {
			continue
		}
		val := entry[key]
		if val == nil {
			continue
		}
		if list, ok := val.([]any); ok {
			directive := convertSshDirectiveKeyToPascalCase(key)
			directive = strings.TrimSuffix(directive, "s")
			for _, item := range list {
				lines = append(lines, "  "+directive+" "+sshDirectiveValueString(item))
			}
			continue
		}
		lines = append(lines, "  "+convertSshDirectiveKeyToPascalCase(key)+" "+sshDirectiveValueString(val))
	}
	return lines, nil
}

func getSshHostBlockContent(item map[string]any) (string, error) {
	defaults := getMap(item, "defaults")
	commentTemplate := getString(item, "comment_template")
	hosts := asList(item["hosts"])

	var blocks []string
	for _, raw := range hosts {
		hostEntry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		entry := mergeSshHostEntry(hostEntry, defaults)
		lines, err := getSshHostBlockEntryLines(entry, commentTemplate)
		if err != nil {
			return "", err
		}
		blocks = append(blocks, strings.Join(lines, "\n"))
	}
	return strings.Join(blocks, "\n\n"), nil
}

func testSshHostBlockPresent(item map[string]any, name string) (bool, error) {
	dest := resolvePath(getString(item, "dest"))
	if !fileExists(dest) {
		return false, nil
	}
	markers := blockInFileMarkers(item, resolveBlockIdentifier(item, name))
	lines := getFileLines(dest)
	r := findBlockRange(lines, markers.Begin, markers.End)
	if r == nil {
		return false, nil
	}
	var existing []string
	if r.EndIndex > r.BeginIndex+1 {
		existing = lines[r.BeginIndex+1 : r.EndIndex]
	}
	content, err := getSshHostBlockContent(item)
	if err != nil {
		return false, err
	}
	desired := getDesiredBlockLines(content)
	existingCanon := strings.Join(canonicalizeSshHostBlockLines(existing), "\n")
	desiredCanon := strings.Join(canonicalizeSshHostBlockLines(desired), "\n")
	return existingCanon == desiredCanon, nil
}

// canonicalizeSshHostBlockLines makes the presence check insensitive to a
// host stanza's directive line order: this handler's own generated order
// is a deterministic alphabetical sort of each entry's extra fields (Go
// map iteration has no natural order to preserve, unlike the original
// ironstate.ps1's ordered hashtables), which won't in general match a
// hand-authored file, or one written before this sort was introduced,
// even when the ssh_config content is fully equivalent - causing a
// perpetual false "would install". Splits into blank-line-separated host
// stanzas; within each stanza, leaves any leading '#'-comment/'Host '/
// 'HostName ' lines in place and sorts every other directive line, so two
// stanzas with the same directives in a different order canonicalize
// identically.
func canonicalizeSshHostBlockLines(lines []string) []string {
	var out []string
	for i, stanza := range splitSshStanzas(lines) {
		if i > 0 {
			out = append(out, "")
		}
		out = append(out, canonicalizeSshStanza(stanza)...)
	}
	return out
}

func splitSshStanzas(lines []string) [][]string {
	var stanzas [][]string
	var current []string
	for _, l := range lines {
		if l == "" {
			if len(current) > 0 {
				stanzas = append(stanzas, current)
				current = nil
			}
			continue
		}
		current = append(current, l)
	}
	if len(current) > 0 {
		stanzas = append(stanzas, current)
	}
	return stanzas
}

func canonicalizeSshStanza(lines []string) []string {
	var anchors, rest []string
	for _, l := range lines {
		trimmed := strings.TrimLeft(l, " \t")
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "Host ") || strings.HasPrefix(trimmed, "HostName ") {
			anchors = append(anchors, l)
			continue
		}
		rest = append(rest, l)
	}
	sort.Strings(rest)
	return append(anchors, rest...)
}

func setSshHostBlock(item map[string]any, name string) error {
	dest := resolvePath(getString(item, "dest"))
	create := getBool(item, "create", false)
	exists := fileExists(dest)
	if !exists && !create {
		engine.Warn("ssh_host_block dest does not exist and 'create' is false, skipping: %s", dest)
		return nil
	}

	markers := blockInFileMarkers(item, resolveBlockIdentifier(item, name))
	var lines []string
	if exists {
		lines = getFileLines(dest)
	}

	content, err := getSshHostBlockContent(item)
	if err != nil {
		return err
	}
	newBlockLines := append([]string{markers.Begin}, append(getDesiredBlockLines(content), markers.End)...)

	if r := findBlockRange(lines, markers.Begin, markers.End); r != nil {
		lines = append(lines[:r.BeginIndex], append(newBlockLines, lines[r.EndIndex+1:]...)...)
	} else {
		insertIndex := getBlockInsertIndex(lines, getStringOr(item, "insertafter", "EOF"), getString(item, "insertbefore"))
		out := make([]string, 0, len(lines)+len(newBlockLines))
		out = append(out, lines[:insertIndex]...)
		out = append(out, newBlockLines...)
		out = append(out, lines[insertIndex:]...)
		lines = out
	}

	if exists && getBool(item, "backup", false) {
		if err := backupBlockInFileDest(dest); err != nil {
			return err
		}
	}
	if err := ensureParentDir(dest); err != nil {
		return err
	}
	return writeBlockInFileLines(dest, lines)
}

func removeSshHostBlock(item map[string]any, name string) error {
	dest := resolvePath(getString(item, "dest"))
	if !fileExists(dest) {
		return nil
	}
	markers := blockInFileMarkers(item, resolveBlockIdentifier(item, name))
	lines := getFileLines(dest)
	r := findBlockRange(lines, markers.Begin, markers.End)
	if r == nil {
		return nil
	}
	if getBool(item, "backup", false) {
		if err := backupBlockInFileDest(dest); err != nil {
			return err
		}
	}
	lines = append(lines[:r.BeginIndex], lines[r.EndIndex+1:]...)
	return writeBlockInFileLines(dest, lines)
}

func (sshHostBlockHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	return testSshHostBlockPresent(item, name)
}

func (sshHostBlockHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	dest := resolvePath(getString(item, "dest"))
	if action == engine.ActionUninstall {
		return "remove ironstate managed ssh host block from " + dest, nil
	}
	return "manage ssh host block in " + dest, nil
}

func (sshHostBlockHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return engine.ExecResult{}, setSshHostBlock(item, name)
}

func (sshHostBlockHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return engine.ExecResult{}, removeSshHostBlock(item, name)
}

// ScanRole implements engine.ScanCapable - discovered packages seed
// roles/packages in a generated playbook (see internal/scan).
func (sshHostBlockHandler) ScanRole() string { return "roles/ssh/config" }

// sshDirectiveValues is the set of raw values a single directive keyword
// took within one Host stanza, plus the case as first written (used to
// canonicalize unrecognized/dynamic directive names back to snake_case).
type sshDirectiveValues struct {
	OriginalKey string
	Values      []string
}

// sshScanEntry is one "Host ..." stanza read from an ssh_config file,
// before it's grouped into ssh_host_block items.
type sshScanEntry struct {
	Dest   string
	Host   string
	Fields map[string]*sshDirectiveValues // keyed by lower-cased directive keyword
}

// sshKnownDirectiveSnakeCase maps common OpenSSH client directive
// keywords (lower-cased) to the snake_case key ssh_host_block's schema
// uses for them - the reverse of convertSshDirectiveKeyToPascalCase for
// the directives this handler is most likely to encounter. Anything not
// listed here falls back to sshDirectiveKeyToSnakeCase, since the
// handler's schema is otherwise fully dynamic.
var sshKnownDirectiveSnakeCase = map[string]string{
	"hostname":                 "host_name",
	"user":                     "user",
	"port":                     "port",
	"identityfile":             "identity_file",
	"identitiesonly":           "identities_only",
	"proxycommand":             "proxy_command",
	"proxyjump":                "proxy_jump",
	"forwardagent":             "forward_agent",
	"forwardx11":               "forward_x11",
	"stricthostkeychecking":    "strict_host_key_checking",
	"userknownhostsfile":       "user_known_hosts_file",
	"serveraliveinterval":      "server_alive_interval",
	"serveralivecountmax":      "server_alive_count_max",
	"compression":              "compression",
	"controlmaster":            "control_master",
	"controlpath":              "control_path",
	"controlpersist":           "control_persist",
	"addkeystoagent":           "add_keys_to_agent",
	"pubkeyauthentication":     "pubkey_authentication",
	"passwordauthentication":   "password_authentication",
	"preferredauthentications": "preferred_authentications",
	"localforward":             "local_forward",
	"remoteforward":            "remote_forward",
	"dynamicforward":           "dynamic_forward",
	"sendenv":                  "send_env",
	"setenv":                   "set_env",
	"certificatefile":          "certificate_file",
	"ciphers":                  "ciphers",
	"macs":                     "macs",
	"kexalgorithms":            "kex_algorithms",
	"hostkeyalgorithms":        "host_key_algorithms",
	"connecttimeout":           "connect_timeout",
	"connectionattempts":       "connection_attempts",
	"tcpkeepalive":             "tcp_keep_alive",
	"batchmode":                "batch_mode",
	"canonicalizehostname":     "canonicalize_hostname",
	"gssapiauthentication":     "gssapi_authentication",
	"loglevel":                 "log_level",
}

// sshDirectiveKeyToSnakeCase converts a CamelCase directive keyword (e.g.
// "HostName", "ServerAliveInterval") to snake_case, for directives not
// found in sshKnownDirectiveSnakeCase - the inverse of
// convertSshDirectiveKeyToPascalCase, used so dynamic/unrecognized
// directives still round-trip through the handler's own writer.
func sshDirectiveKeyToSnakeCase(name string) string {
	var sb strings.Builder
	runes := []rune(name)
	for i, r := range runes {
		if i > 0 && r >= 'A' && r <= 'Z' {
			prev := runes[i-1]
			if (prev >= 'a' && prev <= 'z') || (prev >= '0' && prev <= '9') {
				sb.WriteRune('_')
			}
		}
		sb.WriteRune(unicode.ToLower(r))
	}
	return sb.String()
}

func canonicalSshDirectiveSnakeCase(lowerKey, originalKey string) string {
	if snake, ok := sshKnownDirectiveSnakeCase[lowerKey]; ok {
		return snake
	}
	return sshDirectiveKeyToSnakeCase(originalKey)
}

// convertSshScannedValue turns a raw ssh_config value string into a bool,
// int, or string - mirroring how ssh_host_block's own writer
// (sshDirectiveValueString) renders those same Go types back out.
func convertSshScannedValue(raw string) any {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		raw = raw[1 : len(raw)-1]
	}
	switch strings.ToLower(raw) {
	case "yes":
		return true
	case "no":
		return false
	}
	if n, err := strconv.Atoi(raw); err == nil {
		return n
	}
	return raw
}

// splitSshDirectiveLine splits one ssh_config line into its keyword and
// value, accepting both "Key value" and "Key=value" (with optional
// surrounding whitespace around '=') forms; blank/comment lines return
// ok=false.
func splitSshDirectiveLine(line string) (key, value string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	idx := strings.IndexAny(line, " \t=")
	if idx < 0 {
		return "", "", false
	}
	key = line[:idx]
	rest := strings.TrimSpace(line[idx:])
	rest = strings.TrimPrefix(rest, "=")
	rest = strings.TrimSpace(rest)
	if key == "" || rest == "" {
		return "", "", false
	}
	return key, rest, true
}

// resolveSshIncludePath expands '~' and, for a relative pattern, resolves
// it against the directory of the ssh_config file that referenced it -
// mirroring OpenSSH's own Include semantics.
func resolveSshIncludePath(pattern, baseDir string) string {
	expanded := resolvePath(pattern)
	if filepath.IsAbs(expanded) {
		return expanded
	}
	return filepath.Join(baseDir, expanded)
}

// scanSshConfigFile reads one ssh_config-style file, collecting every
// "Host" stanza's directives into out, and recursing into any "Include"
// target (glob-expanded, relative to this file's own directory) -
// visited guards against an Include cycle re-reading the same file.
func scanSshConfigFile(path string, visited map[string]bool, out *[]sshScanEntry) {
	canon, err := filepath.Abs(path)
	if err != nil {
		canon = path
	}
	if visited[canon] {
		return
	}
	visited[canon] = true
	if !fileExists(path) {
		return
	}

	var current *sshScanEntry
	inHostBlock := false
	flush := func() {
		if current != nil {
			*out = append(*out, *current)
			current = nil
		}
	}
	for _, raw := range getFileLines(path) {
		key, value, ok := splitSshDirectiveLine(raw)
		if !ok {
			continue
		}
		lowerKey := strings.ToLower(key)
		switch lowerKey {
		case "host":
			flush()
			current = &sshScanEntry{Dest: path, Host: value, Fields: map[string]*sshDirectiveValues{}}
			inHostBlock = true
			continue
		case "match":
			flush()
			inHostBlock = false
			continue
		case "include":
			dir := filepath.Dir(path)
			for _, pattern := range strings.Fields(value) {
				resolved := resolveSshIncludePath(pattern, dir)
				matches, _ := filepath.Glob(resolved)
				if len(matches) == 0 && !strings.ContainsAny(resolved, "*?[") {
					matches = []string{resolved}
				}
				sort.Strings(matches)
				for _, m := range matches {
					scanSshConfigFile(m, visited, out)
				}
			}
			continue
		}
		if !inHostBlock || current == nil {
			continue
		}
		dv, exists := current.Fields[lowerKey]
		if !exists {
			dv = &sshDirectiveValues{OriginalKey: key}
			current.Fields[lowerKey] = dv
		}
		dv.Values = append(dv.Values, value)
	}
	flush()
}

// sshConfigBaseFiles are the ssh_config-style files scanned by default:
// the current user's own config, plus the system-wide config (whose
// location differs on Windows vs. everywhere else).
func sshConfigBaseFiles() []string {
	files := []string{resolvePath("~/.ssh/config")}
	if runtime.GOOS == "windows" {
		if programData := os.Getenv("PROGRAMDATA"); programData != "" {
			files = append(files, filepath.Join(programData, "ssh", "ssh_config"))
		}
	} else {
		files = append(files, "/etc/ssh/ssh_config")
	}
	return files
}

// buildSshHostBlockScanItems groups scanned Host stanzas into
// ssh_host_block items: one item per (source file, HostName) pair when a
// stanza defines its own HostName, and one catch-all item per source
// file for every stanza that doesn't.
func buildSshHostBlockScanItems(entries []sshScanEntry) []engine.ScanItem {
	type destGroups struct {
		order  []string
		groups map[string][]map[string]any
	}
	byDest := map[string]*destGroups{}
	var destOrder []string

	for _, e := range entries {
		if strings.TrimSpace(e.Host) == "*" {
			// A bare wildcard stanza holds catch-all defaults, not a
			// real host worth seeding into a playbook.
			continue
		}
		hostEntry := map[string]any{"host": e.Host}
		groupKey := ""
		keys := make([]string, 0, len(e.Fields))
		for k := range e.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, lowerKey := range keys {
			dv := e.Fields[lowerKey]
			snake := canonicalSshDirectiveSnakeCase(lowerKey, dv.OriginalKey)
			if lowerKey == "hostname" {
				groupKey = dv.Values[0]
			}
			if len(dv.Values) > 1 {
				values := make([]any, len(dv.Values))
				for i, v := range dv.Values {
					values[i] = convertSshScannedValue(v)
				}
				hostEntry[snake+"s"] = values
			} else {
				hostEntry[snake] = convertSshScannedValue(dv.Values[0])
			}
		}

		dg, ok := byDest[e.Dest]
		if !ok {
			dg = &destGroups{groups: map[string][]map[string]any{}}
			byDest[e.Dest] = dg
			destOrder = append(destOrder, e.Dest)
		}
		if _, ok := dg.groups[groupKey]; !ok {
			dg.order = append(dg.order, groupKey)
		}
		dg.groups[groupKey] = append(dg.groups[groupKey], hostEntry)
	}

	var items []engine.ScanItem
	for _, dest := range destOrder {
		dg := byDest[dest]
		for _, groupKey := range dg.order {
			hosts := dg.groups[groupKey]
			name := groupKey
			if name == "" {
				name = "hosts (" + filepath.Base(dest) + ")"
			}
			hostsAny := make([]any, len(hosts))
			for i, h := range hosts {
				hostsAny[i] = h
			}
			items = append(items, engine.ScanItem{
				Module: "ssh_host_block",
				Name:   name,
				Config: map[string]any{"dest": dest, "hosts": hostsAny},
				Tags:   []string{"ssh", "hosts"},
			})
		}
	}
	return items
}

// Scan implements engine.ScanCapable: reads the user's and system's
// ssh_config files (following any Include directives) and turns their
// "Host" stanzas into ssh_host_block items, grouped by HostName - see
// buildSshHostBlockScanItems.
func (sshHostBlockHandler) Scan(ctx engine.Context) ([]engine.ScanItem, error) {
	visited := map[string]bool{}
	var entries []sshScanEntry
	for _, base := range sshConfigBaseFiles() {
		scanSshConfigFile(base, visited, &entries)
	}
	return buildSshHostBlockScanItems(entries), nil
}
