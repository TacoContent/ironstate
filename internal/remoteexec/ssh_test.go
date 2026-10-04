package remoteexec

import (
	"encoding/base64"
	"encoding/binary"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
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

func TestPowerShellCommandLineEncodesStructuredInvocation(t *testing.T) {
	program := `C:\Program Files\ironstate\ironstate.exe`
	line, err := PowerShellProgramCommandLine(program, []string{"agent", `a'; Start-Process calc`})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(line, program) || strings.Contains(line, "Start-Process") {
		t.Fatalf("command data leaked into shell text: %s", line)
	}
	encoded := strings.TrimPrefix(line, "powershell.exe -NoLogo -NoProfile -NonInteractive -EncodedCommand ")
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	units := make([]uint16, len(data)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(data[i*2:])
	}
	script := string(utf16.Decode(units))
	if !strings.Contains(script, "ConvertFrom-Json") || !strings.Contains(script, "@a") {
		t.Fatalf("unexpected encoded PowerShell script: %s", script)
	}
}

func TestPowerShellFinalizeUploadVerifiesBeforeMoving(t *testing.T) {
	script := powerShellFinalizeUploadScript(`C:\cache\.upload`, `C:\cache\agent.exe`, strings.Repeat("a", 64))
	if !strings.Contains(script, "Get-FileHash") || !strings.Contains(script, "Move-Item") || !strings.Contains(script, "uploaded agent sha256 mismatch") {
		t.Fatalf("finalize script does not verify the staged agent: %s", script)
	}
}

func TestSFTPBatchPutQuotesPaths(t *testing.T) {
	local := filepath.Join("build dir", `ironstate"agent.exe`)
	got := sftpBatchPut(local, `C:\Users\deploy\AppData\Local\ironstate\agent.exe`)
	localQuoted := strings.ReplaceAll(filepath.ToSlash(local), `"`, `\"`)
	want := "put \"" + localQuoted + "\" \"C:/Users/deploy/AppData/Local/ironstate/agent.exe\"\n"
	if got != want {
		t.Fatalf("SFTP batch = %q, want %q", got, want)
	}
}

func TestSFTPStartupNoiseHint(t *testing.T) {
	got := sftpStartupNoiseHint("Received message too long 1282367844Ensure the remote shell produces no output")
	if !strings.Contains(got, `emitted "Load"`) || !strings.Contains(got, "PowerShell startup scripts") {
		t.Fatalf("hint = %q", got)
	}
	if got := sftpStartupNoiseHint("Connection closed"); got != "" {
		t.Fatalf("unexpected hint = %q", got)
	}
}

func TestAgentVersionTextStripsCLIXML(t *testing.T) {
	output := `#< CLIXMLironstate dev (commit none, built unknown)<Objs Version="1.1.0.1"><Obj S="progress">Preparing modules for first use.</Obj></Objs>`
	if got, want := agentVersionText(output), "ironstate dev (commit none, built unknown)"; got != want {
		t.Fatalf("agentVersionText() = %q, want %q", got, want)
	}
}

func TestRemoteScriptsAreSingleLine(t *testing.T) {
	for name, script := range map[string]string{"probe": probeScript, "check": checkScript, "upload": uploadScript} {
		if strings.ContainsAny(script, "\n\r!") {
			t.Errorf("%s script must be one line without '!': %q", name, script)
		}
	}
}

func TestProbeScriptSupportsEmbeddedLinuxHashCommands(t *testing.T) {
	for _, tool := range []string{"sha=openssl", "sha=busybox", "openssl dgst -sha256", "busybox sha256sum"} {
		if !strings.Contains(probeScript, tool) && !strings.Contains(hashSnippet, tool) {
			t.Errorf("probe/hash scripts do not support %q", tool)
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
		"arm32":        strings.Replace(good, "arch=aarch64", "arch=armv7l", 1),
		"noexec":       strings.Replace(good, "exec=ok\n", "", 1),
		"relative dir": strings.Replace(good, "agent_dir=/root", "agent_dir=root", 1),
	}
	for name, out := range cases {
		if _, err := parseProbe(out); err == nil {
			t.Errorf("%s: parseProbe accepted", name)
		}
	}
	noSHA := strings.Replace(good, "sha=sha256sum\n", "", 1)
	if info, err := parseProbe(noSHA); err != nil || info.SHATool != "" {
		t.Errorf("probe without hash utility = %+v, %v", info, err)
	}
}

func TestParseWindowsProbe(t *testing.T) {
	out := probeBegin + "\nos=windows\narch=AMD64\nuid=deploy\nagent_dir=C:\\Users\\deploy\\AppData\\Local\\ironstate\\agent\nsha=powershell\nsudo=na\nexec=ok\nadmin=yes\n" + probeEnd
	info, err := parseProbe(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Platform() != "windows/amd64" || !info.Admin || info.Sudo != "na" {
		t.Fatalf("info = %+v", info)
	}
	want := `C:\Users\deploy\AppData\Local\ironstate\agent\v1.0.0-0123456789ab\ironstate.exe`
	got := RemoteAgentPath(info, "v1.0.0", LocalAgent{SHA256: "0123456789abcdef"})
	if got != want {
		t.Fatalf("agent path = %q, want %q", got, want)
	}
}

func TestRemoteAgentPathIsVersionAndHashKeyed(t *testing.T) {
	info := ProbeInfo{AgentDir: "/home/u/.cache/ironstate/agent"}
	got := RemoteAgentPath(info, "v1.2.3 dev/x", LocalAgent{SHA256: strings.Repeat("ab", 32)})
	if got != "/home/u/.cache/ironstate/agent/v1.2.3_dev_x-abababababab/ironstate" {
		t.Fatalf("path = %s", got)
	}
}
