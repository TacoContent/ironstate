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
