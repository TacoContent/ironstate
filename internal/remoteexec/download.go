package remoteexec

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ReleaseBaseURL is where release assets are downloaded from; overridable
// for tests.
var ReleaseBaseURL = "https://github.com/TacoContent/ironstate/releases/download"

// maxAgentArchiveBytes bounds a downloaded release archive.
const maxAgentArchiveBytes = 256 << 20

var releaseVersionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+([-+][0-9A-Za-z.+-]+)?$`)

// IsReleaseVersion reports whether v names a published release (so its
// agent can be downloaded); "dev" and snapshot builds can't be.
func IsReleaseVersion(v string) bool {
	return releaseVersionPattern.MatchString(v) && !strings.Contains(v, "SNAPSHOT")
}

var httpClient = &http.Client{Timeout: 5 * time.Minute}

// VerifyChecksumsSignature checks checksums.txt against its cosign bundle.
// The default verifies with cosign when it's on PATH and skips otherwise
// (the archive is still checked against checksums.txt fetched over HTTPS).
var VerifyChecksumsSignature = func(ctx context.Context, checksums, bundle string) error {
	cosign, err := exec.LookPath("cosign")
	if err != nil {
		return nil
	}
	cmd := exec.CommandContext(ctx, cosign, "verify-blob", //nolint:gosec // fixed argv
		"--bundle", bundle,
		"--certificate-identity-regexp", `^https://github\.com/TacoContent/ironstate/\.github/workflows/release\.yml@.*$`,
		"--certificate-oidc-issuer", "https://token.actions.githubusercontent.com",
		checksums)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cosign verify-blob: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// download fetches the release archive for platform, verifies it against
// the release's checksums.txt (and its cosign signature when possible),
// and installs the binary into the controller's agent cache.
func (s *AgentSource) download(ctx context.Context, platform string) (string, error) {
	s.downloadMu.Lock()
	defer s.downloadMu.Unlock()
	dest, err := cachedAgentPath(s.Version, platform)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(dest); err == nil {
		return dest, nil
	}
	goos, goarch, ok := strings.Cut(platform, "/")
	if !ok || (goos != "linux" && goos != "darwin") {
		return "", fmt.Errorf("no downloadable agent for %s", platform)
	}

	work, err := os.MkdirTemp("", "ironstate-agent-download-")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(work) }()

	base, err := url.JoinPath(ReleaseBaseURL, "v"+s.Version)
	if err != nil {
		return "", err
	}
	asset := fmt.Sprintf("ironstate_%s_%s_%s.tar.gz", s.Version, goos, goarch)
	checksums := filepath.Join(work, "checksums.txt")
	if err := fetch(ctx, base+"/checksums.txt", checksums, 1<<20); err != nil {
		return "", err
	}
	bundle := filepath.Join(work, "checksums.txt.sigstore.json")
	if err := fetch(ctx, base+"/checksums.txt.sigstore.json", bundle, 1<<20); err == nil {
		if err := VerifyChecksumsSignature(ctx, checksums, bundle); err != nil {
			return "", err
		}
	}
	want, err := checksumFor(checksums, asset)
	if err != nil {
		return "", err
	}
	archive := filepath.Join(work, asset)
	if err := fetch(ctx, base+"/"+asset, archive, maxAgentArchiveBytes); err != nil {
		return "", err
	}
	got, err := fileSHA256(archive)
	if err != nil {
		return "", err
	}
	if got != want {
		return "", fmt.Errorf("%s sha256 %s does not match checksums.txt (%s)", asset, got, want)
	}
	if err := extractAgent(archive, dest); err != nil {
		return "", err
	}
	return dest, nil
}

func fetch(ctx context.Context, rawURL, dest string, limit int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	if token := os.Getenv("GITHUB_TOKEN"); token != "" && strings.HasPrefix(rawURL, "https://github.com/") {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", rawURL, resp.Status)
	}
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // inside our own temp dir
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, limit+1))
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if n > limit {
		return fmt.Errorf("GET %s: response exceeds %d bytes", rawURL, limit)
	}
	return closeErr
}

func checksumFor(checksumsFile, asset string) (string, error) {
	f, err := os.Open(checksumsFile) //nolint:gosec // inside our own temp dir
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == asset {
			return strings.ToLower(fields[0]), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("checksums.txt has no entry for %s", asset)
}

func fileSHA256(p string) (string, error) {
	f, err := os.Open(p) //nolint:gosec // inside our own temp dir
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// extractAgent copies the top-level 'ironstate' file out of a release
// tar.gz into dest, atomically.
func extractAgent(archive, dest string) error {
	f, err := os.Open(archive) //nolint:gosec // inside our own temp dir
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return errors.New("release archive has no 'ironstate' binary")
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg || path.Clean(hdr.Name) != "ironstate" {
			continue
		}
		if hdr.Size > maxAgentArchiveBytes {
			return errors.New("agent binary in release archive is too large")
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			return err
		}
		tmp, err := os.CreateTemp(filepath.Dir(dest), ".ironstate-*")
		if err != nil {
			return err
		}
		_, copyErr := io.CopyN(tmp, tr, hdr.Size)
		closeErr := tmp.Close()
		if copyErr == nil {
			copyErr = closeErr
		}
		if copyErr == nil {
			copyErr = os.Chmod(tmp.Name(), 0o700) //nolint:gosec // an executable we just verified
		}
		if copyErr == nil {
			copyErr = os.Rename(tmp.Name(), dest)
		}
		if copyErr != nil {
			_ = os.Remove(tmp.Name())
		}
		return copyErr
	}
}
