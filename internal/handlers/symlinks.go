package handlers

import "github.com/TacoContent/ironstate/internal/engine"

// symlinksHandler ports Handlers/Symlinks.psm1: a thin wrapper over
// fileHandler's 'link' type - translates 'src'/'dest' into a
// { path; type: link; src; force } item and delegates every Test/Install/
// Uninstall to it. 'force' defaults to true here (unlike fileHandler's own
// default of false), preserving Symlinks.psm1's original "always replace
// whatever's already at dest" behavior.
type symlinksHandler struct{}

func toFileLinkItem(item map[string]any) map[string]any {
	return map[string]any{
		"path":  getString(item, "dest"),
		"type":  "link",
		"src":   getString(item, "src"),
		"force": getBool(item, "force", true),
		"owner": getString(item, "owner"),
		"group": getString(item, "group"),
		"mode":  item["mode"],
	}
}

func (symlinksHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	return testFileItemPresent(toFileLinkItem(item)), nil
}

func (symlinksHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	dest := resolvePath(getString(item, "dest"))
	src := resolvePath(getString(item, "src"))
	if action == engine.ActionUninstall {
		return "remove symlink " + dest, nil
	}
	return "link " + dest + " -> " + src, nil
}

func (symlinksHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return engine.ExecResult{}, installFileItem(toFileLinkItem(item))
}

func (symlinksHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return engine.ExecResult{}, uninstallFileItem(toFileLinkItem(item))
}
