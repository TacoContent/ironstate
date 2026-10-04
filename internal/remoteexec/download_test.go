package remoteexec

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func releaseArchive(t *testing.T, binary []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, data := range map[string][]byte{"README.md": []byte("readme"), "ironstate": binary} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write(data)
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

// fakeRelease serves a v1.2.3 release; corrupt flips the archive checksum.
func fakeRelease(t *testing.T, corrupt bool) (*int32, []byte) {
	t.Helper()
	binary := []byte("#!/bin/sh\necho fake agent\n")
	archive := releaseArchive(t, binary)
	sum := sha256.Sum256(archive)
	checksum := hex.EncodeToString(sum[:])
	if corrupt {
		checksum = strings.Repeat("0", 64)
	}
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		switch r.URL.Path {
		case "/v1.2.3/checksums.txt":
			_, _ = fmt.Fprintf(w, "%s  ironstate_1.2.3_linux_arm64.tar.gz\n%s  ironstate_1.2.3_windows_amd64.zip\n", checksum, strings.Repeat("1", 64))
		case "/v1.2.3/checksums.txt.sigstore.json":
			_, _ = w.Write([]byte("{}"))
		case "/v1.2.3/ironstate_1.2.3_linux_arm64.tar.gz":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	origURL, origCache, origVerify := ReleaseBaseURL, UserCacheDir, VerifyChecksumsSignature
	ReleaseBaseURL = srv.URL
	cache := t.TempDir()
	UserCacheDir = func() (string, error) { return cache, nil }
	VerifyChecksumsSignature = func(context.Context, string, string) error { return nil }
	t.Cleanup(func() { ReleaseBaseURL, UserCacheDir, VerifyChecksumsSignature = origURL, origCache, origVerify })
	return &hits, binary
}

func TestResolveDownloadsVerifiesAndCachesReleaseAgent(t *testing.T) {
	hits, binary := fakeRelease(t, false)
	src := &AgentSource{Version: "1.2.3"}
	agent, err := src.Resolve(context.Background(), "linux/arm64")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(agent.Path)
	if err != nil || !bytes.Equal(data, binary) {
		t.Fatalf("cached agent = %q, %v", data, err)
	}
	before := atomic.LoadInt32(hits)
	again, err := (&AgentSource{Version: "1.2.3"}).Resolve(context.Background(), "linux/arm64")
	if err != nil || again.Path != agent.Path || atomic.LoadInt32(hits) != before {
		t.Fatalf("second resolve re-downloaded (hits %d -> %d) or failed: %v", before, atomic.LoadInt32(hits), err)
	}
}

func TestResolveRejectsChecksumMismatch(t *testing.T) {
	fakeRelease(t, true)
	_, err := (&AgentSource{Version: "1.2.3"}).Resolve(context.Background(), "linux/arm64")
	if err == nil || !strings.Contains(err.Error(), "does not match checksums.txt") {
		t.Fatalf("err = %v, want checksum mismatch", err)
	}
	if p, _ := cachedAgentPath("1.2.3", "linux/arm64"); fileExists(p) {
		t.Error("unverified agent was cached")
	}
}

func TestResolveRejectsBadSignature(t *testing.T) {
	fakeRelease(t, false)
	VerifyChecksumsSignature = func(context.Context, string, string) error { return errors.New("bad signature") }
	_, err := (&AgentSource{Version: "1.2.3"}).Resolve(context.Background(), "linux/arm64")
	if err == nil || !strings.Contains(err.Error(), "bad signature") {
		t.Fatalf("err = %v, want signature failure", err)
	}
}

func TestResolveNeverDownloadsForDevOrWhenDisabled(t *testing.T) {
	hits, _ := fakeRelease(t, false)
	for _, src := range []*AgentSource{{Version: "dev"}, {Version: "1.2.3-SNAPSHOT-abc"}, {Version: "1.2.3", NoDownload: true}} {
		if _, err := src.Resolve(context.Background(), "linux/arm64"); err == nil || !strings.Contains(err.Error(), "--agent-binary") {
			t.Errorf("%+v: err = %v, want --agent-binary hint", src, err)
		}
	}
	if atomic.LoadInt32(hits) != 0 {
		t.Errorf("made %d requests, want none", atomic.LoadInt32(hits))
	}
}

func TestIsReleaseVersion(t *testing.T) {
	for v, want := range map[string]bool{"1.2.3": true, "0.9.0-rc.1": true, "dev": false, "": false, "v1.2.3": false, "1.2.3-SNAPSHOT-abc": false} {
		if got := IsReleaseVersion(v); got != want {
			t.Errorf("IsReleaseVersion(%q) = %v", v, got)
		}
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
