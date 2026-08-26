package handlers

import (
	"fmt"
	"os"
	"os/user"
	"runtime"
	"strconv"
	"strings"
)

func parseFileModeValue(v any, subject string) (os.FileMode, bool, error) {
	if v == nil {
		return 0, false, nil
	}
	toMode := func(n uint64) (os.FileMode, bool, error) {
		if n > 0o7777 {
			return 0, false, fmt.Errorf("invalid %s mode %o: out of range", subject, n)
		}
		return os.FileMode(n), true, nil
	}
	switch raw := v.(type) {
	case string:
		s := strings.TrimSpace(raw)
		if s == "" {
			return 0, false, nil
		}
		s = strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(s), "0o"), "0")
		if s == "" {
			s = "0"
		}
		parsed, err := strconv.ParseUint(s, 8, 32)
		if err != nil {
			return 0, false, fmt.Errorf("invalid %s mode %q: %w", subject, raw, err)
		}
		return toMode(parsed)
	case int:
		if raw < 0 {
			return 0, false, fmt.Errorf("invalid negative %s mode %d", subject, raw)
		}
		return toMode(uint64(raw))
	case int64:
		if raw < 0 {
			return 0, false, fmt.Errorf("invalid negative %s mode %d", subject, raw)
		}
		return toMode(uint64(raw))
	case float64:
		if raw < 0 {
			return 0, false, fmt.Errorf("invalid negative %s mode %v", subject, raw)
		}
		if raw != float64(int64(raw)) {
			return 0, false, fmt.Errorf("invalid non-integer %s mode %v", subject, raw)
		}
		return toMode(uint64(raw))
	default:
		return 0, false, fmt.Errorf("invalid %s mode type %T", subject, raw)
	}
}

func resolveOwnerID(owner string) (int, error) {
	if n, err := strconv.Atoi(owner); err == nil {
		return n, nil
	}
	u, err := user.Lookup(owner)
	if err != nil {
		return 0, err
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return 0, err
	}
	return uid, nil
}

func resolveGroupID(group string) (int, error) {
	if n, err := strconv.Atoi(group); err == nil {
		return n, nil
	}
	g, err := user.LookupGroup(group)
	if err != nil {
		return 0, err
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return 0, err
	}
	return gid, nil
}

func applyWindowsOwnerGroup(targetPath, owner, group string) error {
	// Set owner/group directly on ACL metadata without altering DACL entries.
	ps := "$p=$args[0];$o=$args[1];$g=$args[2];$acl=Get-Acl -LiteralPath $p;if($o){$acl.SetOwner([System.Security.Principal.NTAccount]$o)};if($g){$acl.SetGroup([System.Security.Principal.NTAccount]$g)};Set-Acl -LiteralPath $p -AclObject $acl"
	res, err := runner.Run("pwsh", []string{"-NoProfile", "-NonInteractive", "-Command", ps, targetPath, owner, group})
	if err != nil {
		return err
	}
	if res.RC != 0 {
		stderr := strings.TrimSpace(res.Stderr)
		if stderr == "" {
			stderr = "Set-Acl failed"
		}
		return fmt.Errorf("set owner/group on %s failed: %s", targetPath, stderr)
	}
	return nil
}

func applyOwnerGroup(targetPath, owner, group string) error {
	if owner == "" && group == "" {
		return nil
	}
	if runtime.GOOS == "windows" {
		return applyWindowsOwnerGroup(targetPath, owner, group)
	}

	uid := -1
	gid := -1
	if owner != "" {
		resolvedUID, err := resolveOwnerID(owner)
		if err != nil {
			return fmt.Errorf("resolve owner %q: %w", owner, err)
		}
		uid = resolvedUID
	}
	if group != "" {
		resolvedGID, err := resolveGroupID(group)
		if err != nil {
			return fmt.Errorf("resolve group %q: %w", group, err)
		}
		gid = resolvedGID
	}
	return os.Chown(targetPath, uid, gid)
}

func applyPathMetadata(path string, item map[string]any) error {
	if mode, ok, err := parseFileModeValue(item["mode"], "path"); err != nil {
		return err
	} else if ok {
		if err := os.Chmod(path, mode); err != nil {
			return err
		}
	}
	owner := strings.TrimSpace(getString(item, "owner"))
	group := strings.TrimSpace(getString(item, "group"))
	return applyOwnerGroup(path, owner, group)
}

func hasPathMetadataDirective(item map[string]any) bool {
	if strings.TrimSpace(getString(item, "owner")) != "" {
		return true
	}
	if strings.TrimSpace(getString(item, "group")) != "" {
		return true
	}
	v, ok := item["mode"]
	if !ok || v == nil {
		return false
	}
	if s, isString := v.(string); isString {
		return strings.TrimSpace(s) != ""
	}
	return true
}
