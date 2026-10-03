package remoteexec

import (
	"strings"
	"testing"
)

func TestParseSSHTarget(t *testing.T) {
	ok := map[string]SSHTarget{
		"host":               {Host: "host"},
		"user@host":          {User: "user", Host: "host"},
		"user@host.lan:2222": {User: "user", Host: "host.lan", Port: 2222},
		"[::1]:22":           {Host: "::1", Port: 22},
		"deploy@[fe80::1]":   {User: "deploy", Host: "fe80::1"},
		"my_alias":           {Host: "my_alias"},
	}
	for in, want := range ok {
		got, err := ParseSSHTarget(in)
		if err != nil || got != want {
			t.Errorf("ParseSSHTarget(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "-oProxyCommand=evil", "user@-oProxyCommand=x", "-l@host", "host:0", "host:99999", "host:abc", "ho st", "host;rm", "[::1", "a@b@c d"} {
		if _, err := ParseSSHTarget(bad); err == nil {
			t.Errorf("ParseSSHTarget(%q) accepted", bad)
		}
	}
}

func TestSSHArgsEndWithSeparatorAndHost(t *testing.T) {
	tr := &SSHTransport{Target: SSHTarget{User: "u", Host: "h", Port: 2200}, Options: SSHOptions{ConfigFile: "/cfg", AcceptNewHostKeys: true}}
	args := tr.baseArgs()
	joined := strings.Join(args, " ")
	if args[len(args)-2] != "--" || args[len(args)-1] != "h" {
		t.Fatalf("args must end with -- host: %v", args)
	}
	for _, want := range []string{"-F /cfg", "BatchMode=yes", "StrictHostKeyChecking=accept-new", "-p 2200", "-l u", "-T"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q: %s", want, joined)
		}
	}
	if strings.Contains(joined, "StrictHostKeyChecking=no") {
		t.Error("host key checking must never be disabled")
	}
}

func TestPosixCommandLineQuotesEveryWord(t *testing.T) {
	got, err := PosixCommandLine(RemoteCommand{Program: "/home/o'brien/ironstate", Args: []string{"agent", "$(rm -rf /)", "a b"}})
	if err != nil {
		t.Fatal(err)
	}
	want := `'/home/o'\''brien/ironstate' 'agent' '$(rm -rf /)' 'a b'`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	if _, err := PosixCommandLine(RemoteCommand{Program: "x", Args: []string{"a\nb"}}); err == nil {
		t.Error("newline accepted")
	}
}

func TestRemoteScriptsAreSingleLine(t *testing.T) {
	for name, script := range map[string]string{"probe": probeScript, "check": checkScript, "upload": uploadScript} {
		if strings.ContainsAny(script, "\n\r!") {
			t.Errorf("%s script must be one line without '!': %q", name, script)
		}
	}
}

func TestParseProbe(t *testing.T) {
	good := "Welcome!\n" + probeBegin + "\nos=Linux\narch=aarch64\nuid=0\nagent_dir=/root/.cache/ironstate/agent\nsha=sha256sum\nsudo=nopasswd\nexec=ok\n" + probeEnd + "\n"
	info, err := parseProbe(good)
	if err != nil {
		t.Fatal(err)
	}
	if info.Platform() != "linux/arm64" || info.Sudo != "nopasswd" || info.SHATool != "sha256sum" {
		t.Fatalf("info = %+v", info)
	}
	cases := map[string]string{
		"no sentinels": "hello\n",
		"windows":      strings.Replace(good, "os=Linux", "os=MINGW64_NT", 1),
		"arm32":        strings.Replace(good, "arch=aarch64", "arch=armv7l", 1),
		"no sha tool":  strings.Replace(good, "sha=sha256sum\n", "", 1),
		"noexec":       strings.Replace(good, "exec=ok\n", "", 1),
		"relative dir": strings.Replace(good, "agent_dir=/root", "agent_dir=root", 1),
	}
	for name, out := range cases {
		if _, err := parseProbe(out); err == nil {
			t.Errorf("%s: parseProbe accepted", name)
		}
	}
}

func TestRemoteAgentPathIsVersionAndHashKeyed(t *testing.T) {
	info := ProbeInfo{AgentDir: "/home/u/.cache/ironstate/agent"}
	got := RemoteAgentPath(info, "v1.2.3 dev/x", LocalAgent{SHA256: strings.Repeat("ab", 32)})
	if got != "/home/u/.cache/ironstate/agent/v1.2.3_dev_x-abababababab/ironstate" {
		t.Fatalf("path = %s", got)
	}
}
