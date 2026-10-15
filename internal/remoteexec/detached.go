package remoteexec

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/TacoContent/ironstate/internal/remoteexec/protocol"
)

// StartDetached stages the job for the target agent and returns once the
// background process has started. The agent removes the staged job after
// loading it and writes its result to the normal run log.
func StartDetached(ctx context.Context, t Transport, agentPath string, info ProbeInfo, job *PreparedJob) error {
	if _, ok := t.(FileUploader); !ok {
		return errors.New("detached apply requires an SSH transport with SFTP support")
	}
	if len(job.Job.Env) > 0 || len(job.Job.SecretEnv) > 0 || job.Job.BecomePassword != "" {
		return errors.New("--remote-detach cannot stage .env, .secrets, forwarded environment, or become passwords on the target")
	}
	spool, err := createDetachedSpool(job)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(spool) }()

	spoolDir := joinRemotePath(info.OS, info.StateDir, "detached")
	spoolPath := joinRemotePath(info.OS, spoolDir, job.Job.RunID+".job")
	if info.OS == "windows" {
		if _, err := execCapture(ctx, t, "prepare detached job", powerShellScript(powerShellPrepareUploadScript(spoolDir)), nil); err != nil {
			return err
		}
	} else {
		script := `umask 077; mkdir -p "$1"; chmod 700 "$1"`
		if _, err := execCapture(ctx, t, "prepare detached job", shCommand(script, spoolDir), nil); err != nil {
			return err
		}
	}
	uploader := t.(FileUploader)
	if err := uploader.UploadFile(ctx, spool, spoolPath); err != nil {
		_ = removeRemoteFile(ctx, t, info.OS, spoolPath)
		return fmt.Errorf("upload detached job: %w", err)
	}
	var launch RemoteCommand
	if info.OS == "windows" {
		launch = detachedPowerShellCommand(agentPath, spoolPath)
	} else {
		launch = shCommand(`nohup "$1" agent --protocol 1 --detached --detached-job-file "$2" < "$2" >/dev/null 2>&1 & printf '%s\n' "$!"`, agentPath, spoolPath)
	}
	out, err := execCapture(ctx, t, "start detached agent", launch, nil)
	if err != nil {
		_ = removeRemoteFile(ctx, t, info.OS, spoolPath)
		return err
	}
	if strings.TrimSpace(out) == "" {
		return errors.New("detached agent launch returned no process id")
	}
	return nil
}

func createDetachedSpool(job *PreparedJob) (string, error) {
	f, err := os.CreateTemp("", "ironstate-detached-*.job")
	if err != nil {
		return "", err
	}
	name := f.Name()
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := protocol.WriteJob(f, job.Job); err != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return "", err
	}
	bundle, err := os.Open(job.BundlePath)
	if err == nil {
		_, err = io.Copy(f, bundle)
		_ = bundle.Close()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}

func detachedPowerShellCommand(agentPath, jobPath string) RemoteCommand {
	encode := func(value string) string { return base64String(value) }
	return powerShellScript(`$ErrorActionPreference='Stop'; $agent=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + encode(agentPath) + `')); ` +
		`$job=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + encode(jobPath) + `')); ` +
		`$launchArgs='agent --protocol 1 --detached --detached-job-file "'+$job+'"'; ` +
		`$p=Start-Process -FilePath $agent -ArgumentList $launchArgs -RedirectStandardInput $job -RedirectStandardOutput 'NUL' -RedirectStandardError 'NUL' -WindowStyle Hidden -PassThru; ` +
		`[Console]::Out.Write([string]$p.Id)`)
}

func base64String(value string) string { return base64.StdEncoding.EncodeToString([]byte(value)) }

func removeRemoteFile(ctx context.Context, t Transport, goos, file string) error {
	var cmd RemoteCommand
	if goos == "windows" {
		encoded := base64String(file)
		cmd = powerShellScript(`$p=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + encoded + `')); Remove-Item -LiteralPath $p -Force -ErrorAction SilentlyContinue`)
	} else {
		cmd = shCommand(`rm -f -- "$1"`, file)
	}
	_, err := execCapture(ctx, t, "remove detached job", cmd, nil)
	return err
}
