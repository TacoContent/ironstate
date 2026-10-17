package remoteexec

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func writeInventory(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "inventory.yml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

const sampleInventory = `
defaults:
  user: rconr
  port: 22
hosts:
  snoke: { address: snoke.lan }
  kresh: { address: 10.0.0.12, user: admin, port: 2222 }
  krayt: { platform: windows }
  v6:    { address: "fe80::1" }
  self:  { address: local }
groups:
  linux: [snoke, kresh]
`

func TestLoadInventoryAppliesDefaults(t *testing.T) {
	inv, err := LoadInventory(writeInventory(t, sampleInventory))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"snoke": "rconr@snoke.lan:22",
		"kresh": "admin@10.0.0.12:2222",
		"krayt": "rconr@krayt:22",
		"v6":    "rconr@[fe80::1]:22",
		"self":  "local",
	}
	for name, addr := range want {
		if got := inv.hosts[name].Address(); got != addr {
			t.Errorf("%s address = %q, want %q", name, got, addr)
		}
	}
	if inv.hosts["krayt"].Platform != "windows" || !inv.hosts["self"].Local {
		t.Errorf("hosts = %+v", inv.hosts)
	}
}

func TestLoadInventoryMergesDirectoryInPrimaryThenAlphabeticalOrder(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"main.yml": `
defaults:
  user: primary
  agent_dir: /primary
hosts:
  shared: { address: primary.lan, user: primary }
  primary: { address: primary.lan }
groups:
  core: [primary]
`,
		"00-extra.yaml": `
defaults:
  user: early
  agent_dir: /early
hosts:
  early: { address: early.lan }
`,
		"a-extra.yml": `
defaults:
  port: 2200
hosts:
  shared: { platform: linux }
  alpha: { address: alpha.lan }
groups:
  web: [shared]
`,
		"z-extra.yaml": `
defaults:
  user: late
hosts:
  shared: { address: final.lan }
  zulu: { address: zulu.lan }
groups:
  web: [zulu]
`,
		"ignored.txt": "not: an inventory\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	inv, err := LoadInventory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := inv.hosts["shared"].Address(); got != "primary@final.lan:2200" {
		t.Errorf("shared address = %q, want primary@final.lan:2200", got)
	}
	if got := inv.hosts["shared"].Platform; got != "linux" {
		t.Errorf("shared platform = %q, want linux", got)
	}
	if got := inv.hosts["shared"].AgentDir; got != "/early" {
		t.Errorf("shared agent directory = %q, want /early", got)
	}
	if got := inv.hosts["primary"].Address(); got != "late@primary.lan:2200" {
		t.Errorf("primary host address = %q, want late@primary.lan:2200", got)
	}
	if got := inv.hosts["alpha"].Address(); got != "late@alpha.lan:2200" {
		t.Errorf("alpha host address = %q, want late@alpha.lan:2200", got)
	}
	if got := inv.hosts["zulu"].Address(); got != "late@zulu.lan:2200" {
		t.Errorf("zulu host address = %q, want late@zulu.lan:2200", got)
	}
	web, err := inv.Select([]string{"web"})
	if err != nil || len(web) != 2 || web[0].Name != "shared" || web[1].Name != "zulu" {
		t.Errorf("web group = %+v, %v; want shared,zulu", web, err)
	}
}

func TestLoadInventoryDirectoryUsesMainYAMLWhenMainYMLIsAbsent(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"main.yaml": "defaults:\n  user: primary\nhosts:\n  main: { address: main.lan }\n",
		"a.yml":     "defaults:\n  port: 2222\nhosts:\n  extra: { address: extra.lan }\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	inv, err := LoadInventory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := inv.hosts["main"].Address(); got != "primary@main.lan:2222" {
		t.Errorf("main host address = %q, want primary@main.lan:2222", got)
	}
	if got := inv.hosts["extra"].Address(); got != "primary@extra.lan:2222" {
		t.Errorf("extra host address = %q, want primary@extra.lan:2222", got)
	}
}

func TestLoadInventoryDirectoryRejectsInvalidFragment(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"main.yml":  "hosts:\n  host: { address: host.lan }\n",
		"extra.yml": "hosts:\n  host: { adress: typo }\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := LoadInventory(dir); err == nil || !strings.Contains(err.Error(), "extra.yml") {
		t.Fatalf("LoadInventory error = %v, want invalid fragment filename", err)
	}
}

func TestInventoryBecomeFalseDisablesPlaybookBecome(t *testing.T) {
	inventory := "defaults:\n  become: false\nhosts:\n  router: {address: router.lan}\n  server: {address: server.lan, become: true}\n  admin: {address: admin.lan, become: false}\n"
	inv, err := LoadInventory(writeInventory(t, inventory))
	if err != nil {
		t.Fatal(err)
	}
	if !inv.hosts["router"].DisableBecome || inv.hosts["server"].DisableBecome || !inv.hosts["admin"].DisableBecome {
		t.Fatalf("become overrides: router=%t server=%t admin=%t", inv.hosts["router"].DisableBecome, inv.hosts["server"].DisableBecome, inv.hosts["admin"].DisableBecome)
	}
}

func TestInventoryAgentVerificationDefaultsOnAndCanBeDisabled(t *testing.T) {
	inventory := "defaults:\n  verify_agent: false\nhosts:\n  router: {address: router.lan}\n  server: {address: server.lan, verify_agent: true}\n  localhash: {address: localhash.lan, verify_agent: false}\n"
	inv, err := LoadInventory(writeInventory(t, inventory))
	if err != nil {
		t.Fatal(err)
	}
	if !inv.hosts["router"].SkipAgentVerification || inv.hosts["server"].SkipAgentVerification || !inv.hosts["localhash"].SkipAgentVerification {
		t.Fatalf("verification overrides: router=%t server=%t localhash=%t", inv.hosts["router"].SkipAgentVerification, inv.hosts["server"].SkipAgentVerification, inv.hosts["localhash"].SkipAgentVerification)
	}

	defaultInventory, err := LoadInventory(writeInventory(t, "hosts:\n  strict: {address: strict.lan}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if defaultInventory.hosts["strict"].SkipAgentVerification {
		t.Fatal("verification should be enabled by default")
	}
}

func TestInventorySelect(t *testing.T) {
	inv, err := LoadInventory(writeInventory(t, sampleInventory))
	if err != nil {
		t.Fatal(err)
	}
	names := func(hosts []Host) string {
		var out []string
		for _, h := range hosts {
			out = append(out, h.Name)
		}
		return strings.Join(out, ",")
	}
	cases := map[string]string{
		"":                 "krayt,kresh,self,snoke,v6",
		"linux":            "snoke,kresh",
		"kresh,linux,self": "kresh,snoke,self",
		"all":              "krayt,kresh,self,snoke,v6",
	}
	for limit, want := range cases {
		var l []string
		if limit != "" {
			l = strings.Split(limit, ",")
		}
		hosts, err := inv.Select(l)
		if err != nil || names(hosts) != want {
			t.Errorf("Select(%q) = %s, %v; want %s", limit, names(hosts), err, want)
		}
	}
	if _, err := inv.Select([]string{"nope"}); err == nil {
		t.Error("unknown --limit accepted")
	}
}

func TestLoadInventoryRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"unknown key":       "hosts:\n  a: { adress: x }\n",
		"bad user":          "hosts:\n  a: { user: '-oProxyCommand=x' }\n",
		"bad address":       "hosts:\n  a: { address: '-oProxyCommand=x' }\n",
		"bad platform":      "hosts:\n  a: { platform: plan9 }\n",
		"unknown member":    "hosts:\n  a: {}\ngroups:\n  g: [b]\n",
		"group clashes":     "hosts:\n  a: {}\ngroups:\n  a: [a]\n",
		"reserved all host": "hosts:\n  all: {}\n",
	}
	for name, content := range cases {
		if _, err := LoadInventory(writeInventory(t, content)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if inv, err := LoadInventory(writeInventory(t, "")); err != nil || len(inv.hosts) != 0 {
		t.Errorf("empty inventory: %v", err)
	}
}

func TestForEachHostBoundsConcurrencyAndKeepsOrder(t *testing.T) {
	hosts := make([]Host, 8)
	for i := range hosts {
		hosts[i] = Host{Name: string(rune('a' + i))}
	}
	var running, peak int32
	var mu sync.Mutex
	reports := ForEachHost(hosts, 3, func(h Host) HostReport {
		n := atomic.AddInt32(&running, 1)
		mu.Lock()
		if n > peak {
			peak = n
		}
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&running, -1)
		return HostReport{Name: h.Name}
	})
	if peak > 3 || peak < 2 {
		t.Errorf("peak concurrency = %d, want 2..3", peak)
	}
	for i, r := range reports {
		if r.Name != hosts[i].Name {
			t.Fatalf("report %d = %s, want %s", i, r.Name, hosts[i].Name)
		}
	}
}
