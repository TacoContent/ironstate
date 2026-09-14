package pluginhost

import "testing"

func TestPluginInstallTargetUsesModuleRoot(t *testing.T) {
	module := pluginModulePath("camalot", "hosts")
	if module != "github.com/camalot/ironstate-handler-hosts" {
		t.Fatalf("pluginModulePath = %q", module)
	}

	target := pluginInstallTarget(module, "v1.0.0")
	if target != "github.com/camalot/ironstate-handler-hosts@v1.0.0" {
		t.Fatalf("pluginInstallTarget = %q", target)
	}
}
