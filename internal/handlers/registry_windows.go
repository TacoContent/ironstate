//go:build windows

package handlers

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/sys/windows/registry"

	"github.com/TacoContent/ironstate/internal/engine"
)

// registryHandler ports Handlers/Registry.psm1: writes one or more named
// values under a single registry key. 'state' present/latest requires
// every named value to exist with the exact type+data (drift gets
// corrected); 'absent' only needs any named value to still exist, and
// only removes the named values, never the key itself.
type registryHandler struct{}

var registryHiveAliases = map[string]uint32{
	"HKLM": hive(registry.LOCAL_MACHINE), "HKEY_LOCAL_MACHINE": hive(registry.LOCAL_MACHINE),
	"HKCU": hive(registry.CURRENT_USER), "HKEY_CURRENT_USER": hive(registry.CURRENT_USER),
	"HKCR": hive(registry.CLASSES_ROOT), "HKEY_CLASSES_ROOT": hive(registry.CLASSES_ROOT),
	"HKU": hive(registry.USERS), "HKEY_USERS": hive(registry.USERS),
	"HKCC": hive(registry.CURRENT_CONFIG), "HKEY_CURRENT_CONFIG": hive(registry.CURRENT_CONFIG),
}

func hive(k registry.Key) uint32 { return uint32(k) } //nolint:gosec // every registry.*_ROOT/*_USER/etc. constant is a small positive Win32 pseudo-handle (0x80000000-0x80000006), always representable in uint32

var registryValidTypes = map[string]bool{
	"String": true, "ExpandString": true, "Binary": true, "DWord": true, "MultiString": true, "QWord": true,
}

// resolveRegistryPath ports Resolve-RegistryPath: splits a hive alias
// (with or without a trailing ':', '/' or '\' separators) from the rest
// of the path.
func resolveRegistryPath(path string) (registry.Key, string, error) {
	normalized := strings.TrimLeft(strings.ReplaceAll(path, "/", `\`), `\`)
	splitIndex := strings.Index(normalized, `\`)
	hiveToken := normalized
	rest := ""
	if splitIndex >= 0 {
		hiveToken = normalized[:splitIndex]
		rest = normalized[splitIndex+1:]
	}
	hiveToken = strings.ToUpper(strings.TrimSuffix(hiveToken, ":"))

	root, ok := registryHiveAliases[hiveToken]
	if !ok {
		return 0, "", fmt.Errorf("unknown registry hive '%s' in path '%s' (expected one of: HKLM, HKCU, HKCR, HKU, HKCC, or their HKEY_* full names)", hiveToken, path)
	}
	return registry.Key(root), rest, nil
}

func convertRegistryValueType(t string) (string, error) {
	for valid := range registryValidTypes {
		if strings.EqualFold(valid, t) {
			return valid, nil
		}
	}
	return "", fmt.Errorf("unknown registry value type '%s' (expected one of: String, ExpandString, Binary, DWord, MultiString, QWord)", t)
}

func registryValueSpecs(item map[string]any) []map[string]any {
	var out []map[string]any
	for _, raw := range asList(item["values"]) {
		if m, ok := raw.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func setRegistryValue(k registry.Key, name, valueType string, value any) error {
	switch valueType {
	case "DWord":
		n, err := toInt64(value)
		if err != nil {
			return err
		}
		return k.SetDWordValue(name, uint32(n)) //nolint:gosec // a DWord registry value is inherently 32-bit; truncation on overflow matches the original PowerShell's own '[int32] $Value' cast
	case "QWord":
		n, err := toInt64(value)
		if err != nil {
			return err
		}
		return k.SetQWordValue(name, uint64(n)) //nolint:gosec // a QWord registry value is inherently 64-bit; sign reinterpretation matches the original's '[int64] $Value' cast
	case "Binary":
		data, err := toByteSlice(value)
		if err != nil {
			return err
		}
		return k.SetBinaryValue(name, data)
	case "MultiString":
		return k.SetStringsValue(name, toStringSlice(value))
	case "ExpandString":
		return k.SetExpandStringValue(name, fmt.Sprintf("%v", value))
	default:
		return k.SetStringValue(name, fmt.Sprintf("%v", value))
	}
}

func toInt64(v any) (int64, error) {
	switch val := v.(type) {
	case float64:
		return int64(val), nil
	case int:
		return int64(val), nil
	case int64:
		return val, nil
	case uint64:
		return int64(val), nil //nolint:gosec // reads back a value this same code wrote as int64/uint32/uint64; DWord/QWord width is already enforced at write time
	case string:
		return strconv.ParseInt(val, 10, 64)
	default:
		return 0, fmt.Errorf("cannot convert %v (%T) to an integer registry value", v, v)
	}
}

func toByteSlice(v any) ([]byte, error) {
	list := asList(v)
	out := make([]byte, len(list))
	for i, raw := range list {
		n, err := toInt64(raw)
		if err != nil {
			return nil, err
		}
		if n < 0 || n > 255 {
			return nil, fmt.Errorf("registry Binary value entry %d (%d) is out of byte range 0-255", i, n)
		}
		out[i] = byte(n)
	}
	return out, nil
}

func toStringSlice(v any) []string {
	list := asList(v)
	out := make([]string, len(list))
	for i, raw := range list {
		out[i] = fmt.Sprintf("%v", raw)
	}
	return out
}

func testRegistryValueEqual(current any, desired any, valueType string) bool {
	switch valueType {
	case "Binary":
		a, aok := current.([]byte)
		b, err := toByteSlice(desired)
		if !aok || err != nil || len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	case "MultiString":
		a, aok := current.([]string)
		b := toStringSlice(desired)
		if !aok || len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	case "DWord", "QWord":
		a, aErr := toInt64(current)
		b, bErr := toInt64(desired)
		return aErr == nil && bErr == nil && a == b
	default:
		return fmt.Sprintf("%v", current) == fmt.Sprintf("%v", desired)
	}
}

func readRegistryRawValue(k registry.Key, name string) (any, string, error) {
	// ExpandString values would otherwise auto-expand on read - compare
	// raw string data instead, same rationale as Get-RawRegistryValue.
	if s, valType, err := k.GetStringValue(name); err == nil {
		kind := "String"
		if valType == registry.EXPAND_SZ {
			kind = "ExpandString"
		}
		return s, kind, nil
	}
	if n, valType, err := k.GetIntegerValue(name); err == nil {
		kind := "DWord"
		if valType == registry.QWORD {
			kind = "QWord"
		}
		return int64(n), kind, nil //nolint:gosec // DWord/QWord are read back for comparison only, matching Get-RawRegistryValue's own untyped read
	}
	if ss, _, err := k.GetStringsValue(name); err == nil {
		return ss, "MultiString", nil
	}
	if data, _, err := k.GetBinaryValue(name); err == nil {
		return data, "Binary", nil
	}
	return nil, "", fmt.Errorf("value '%s' not found", name)
}

func (registryHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	root, sub, err := resolveRegistryPath(getString(item, "path"))
	if err != nil {
		return false, err
	}
	specs := registryValueSpecs(item)

	k, err := registry.OpenKey(root, sub, registry.QUERY_VALUE)
	if err != nil {
		return false, nil //nolint:nilerr // key missing means "not installed", not an error
	}
	defer func() { _ = k.Close() }()

	if itemState(item) == "absent" {
		for _, spec := range specs {
			if _, _, err := readRegistryRawValue(k, getString(spec, "name")); err == nil {
				return true, nil
			}
		}
		return false, nil
	}

	for _, spec := range specs {
		valueType, err := convertRegistryValueType(getString(spec, "type"))
		if err != nil {
			return false, err
		}
		existing, existingType, err := readRegistryRawValue(k, getString(spec, "name"))
		if err != nil {
			return false, nil
		}
		if existingType != valueType {
			return false, nil
		}
		if !testRegistryValueEqual(existing, spec["value"], valueType) {
			return false, nil
		}
	}
	return true, nil
}

func (registryHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	keyPath := getString(item, "path")
	var names []string
	for _, spec := range registryValueSpecs(item) {
		names = append(names, getString(spec, "name"))
	}
	joined := strings.Join(names, ", ")
	if action == engine.ActionUninstall {
		return fmt.Sprintf("remove registry value(s) [%s] under %s", joined, keyPath), nil
	}
	return fmt.Sprintf("set registry value(s) [%s] under %s", joined, keyPath), nil
}

func (registryHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	root, sub, err := resolveRegistryPath(getString(item, "path"))
	if err != nil {
		return engine.ExecResult{}, err
	}
	k, _, err := registry.CreateKey(root, sub, registry.SET_VALUE)
	if err != nil {
		return engine.ExecResult{}, err
	}
	defer func() { _ = k.Close() }()

	for _, spec := range registryValueSpecs(item) {
		valueType, err := convertRegistryValueType(getString(spec, "type"))
		if err != nil {
			return engine.ExecResult{}, err
		}
		if err := setRegistryValue(k, getString(spec, "name"), valueType, spec["value"]); err != nil {
			return engine.ExecResult{}, err
		}
	}
	return engine.ExecResult{}, nil
}

func (registryHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	root, sub, err := resolveRegistryPath(getString(item, "path"))
	if err != nil {
		return engine.ExecResult{}, err
	}
	k, err := registry.OpenKey(root, sub, registry.SET_VALUE)
	if err != nil {
		return engine.ExecResult{}, nil //nolint:nilerr // key already gone: nothing to remove
	}
	defer func() { _ = k.Close() }()

	for _, spec := range registryValueSpecs(item) {
		_ = k.DeleteValue(getString(spec, "name"))
	}
	return engine.ExecResult{}, nil
}
