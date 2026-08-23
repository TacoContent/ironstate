package handlers

import "github.com/TacoContent/ironstate/internal/engine"

// rubyGemHandler ports Handlers/RubyGem.psm1 (Ruby Gems). Not present in
// site.schema.json (a pre-existing schema/docs drift bug in the original
// repo, see docs/plans/go-rewrite.md §2/§11) but has real usage
// (roles/languages/ruby/main.yml), so it's implemented here regardless.
type rubyGemHandler struct{}

func (rubyGemHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	pkg := getString(item, "package")
	result, err := runner.Run("gem", []string{"list", pkg, "-i"})
	if err != nil {
		return false, nil //nolint:nilerr // any invocation failure here just means "not installed"
	}
	return result.RC == 0, nil
}

func (rubyGemHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	pkg := getString(item, "package")
	state := itemState(item)
	if action == engine.ActionUninstall {
		return "gem uninstall " + pkg, nil
	}
	if state == "latest" {
		return "gem update " + pkg, nil
	}
	desc := "gem install " + pkg
	if version := getString(item, "version"); version != "" {
		desc += " --version=" + version
	}
	return desc, nil
}

func (rubyGemHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	pkg := getString(item, "package")
	state := itemState(item)
	verb := "install"
	if state == "latest" {
		verb = "update"
	}
	args := []string{verb, pkg}
	if state != "latest" {
		if version := getString(item, "version"); version != "" {
			args = append(args, "--version="+version)
		}
	}
	result := runExternalCommand("gem", args)
	if result.RC != 0 {
		engine.Warn("gem %s %s exited with code %d", verb, pkg, result.RC)
	}
	return result, nil
}

func (rubyGemHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	pkg := getString(item, "package")
	result := runExternalCommand("gem", []string{"uninstall", pkg})
	if result.RC != 0 {
		engine.Warn("gem uninstall %s exited with code %d", pkg, result.RC)
	}
	return result, nil
}
