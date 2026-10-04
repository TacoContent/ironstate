package remoteexec

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/TacoContent/ironstate/internal/model"
	"github.com/TacoContent/ironstate/internal/packages"
	"github.com/TacoContent/ironstate/internal/pathutil"
	"github.com/TacoContent/ironstate/internal/remote"
	"github.com/TacoContent/ironstate/internal/remoteexec/bundle"
	"github.com/TacoContent/ironstate/internal/remoteexec/protocol"
)

// Bundle layout, relative to the agent's working directory.
const (
	bundlePlaybookDir = "playbook"
	bundleFiltersDir  = "filters"
	bundleVarsDir     = "vars-files"
	bundleConfigFile  = "ironstate.yaml"
)

// JobSpec is the controller-side input for one remote run, mirroring the
// local apply flags. Relative paths resolve against the controller's cwd,
// exactly as a local run would.
type JobSpec struct {
	Playbook           string
	VarsFiles          []string
	VarOverrides       []string
	Tags               []string
	Apply              bool
	Verbose            bool
	FiltersDir         string
	FilterInterpreters map[string][]string
	// EnvFile and SecretsFile default to ".env" and ".secrets".
	EnvFile           string
	SecretsFile       string
	ControllerVersion string
	// ForwardEnv adds controller environment values to the job's env.
	ForwardEnv map[string]string
	// AllowRemoteUses pre-approves non-isolated git 'uses:' sources instead
	// of prompting on the controller (--allow-remote-uses).
	AllowRemoteUses bool
	// AllowPluginInstall is forwarded so agents may install missing plugins.
	AllowPluginInstall bool
}

// PreparedJob is a job header plus its bundle, built once and reusable
// across hosts. Close removes the bundle file.
type PreparedJob struct {
	Job        protocol.Job
	BundlePath string
	// Features found by a static scan of the playbook's YAML files.
	UsesBecome bool
	Plugins    []model.Plugin
	// PrefetchedUses describes the git 'uses:' sources shipped in the bundle.
	PrefetchedUses []string

	uses []usesRef
	// root is the controller's playbook dir (for ironstate.lock.yaml).
	root string
}

// usesRef is one literal git 'uses:' found by the static scan.
type usesRef struct {
	Remote, Ref, Path string
	Isolated, Trusted bool
}

// Close removes the temporary bundle.
func (p *PreparedJob) Close() error {
	if p == nil || p.BundlePath == "" {
		return nil
	}
	return os.Remove(p.BundlePath)
}

// Prepare resolves the playbook, builds the bundle into a temp file, and
// reads .env/.secrets values into the job header (never into the bundle).
func Prepare(spec JobSpec) (*PreparedJob, error) {
	resolved, err := packages.ResolvePlaybookPath(spec.Playbook)
	if err != nil {
		return nil, err
	}
	root, err := filepath.Abs(filepath.Dir(resolved))
	if err != nil {
		return nil, err
	}
	bspec := bundle.Spec{
		Dirs:      []bundle.Dir{{Source: root, Dest: bundlePlaybookDir}},
		Generated: map[string][]byte{},
	}
	opts := protocol.JobOptions{
		Playbook:           path.Join(bundlePlaybookDir, filepath.Base(resolved)),
		VarOverrides:       spec.VarOverrides,
		Tags:               spec.Tags,
		Apply:              spec.Apply,
		Verbose:            spec.Verbose,
		AllowPluginInstall: spec.AllowPluginInstall,
	}
	prepared := &PreparedJob{root: root}
	scanPlaybook(prepared, root, resolved)
	approved, err := prefetchUses(prepared, &bspec, spec.AllowRemoteUses)
	if err != nil {
		return nil, err
	}

	filtersSetting, err := bundleFilters(&bspec, root, spec.FiltersDir)
	if err != nil {
		return nil, err
	}
	config := map[string]any{"filters": map[string]any{"dir": filtersSetting}}
	if len(spec.FilterInterpreters) > 0 {
		config["filters"].(map[string]any)["interpreters"] = spec.FilterInterpreters
	}
	configData, err := yaml.Marshal(config)
	if err != nil {
		return nil, err
	}
	bspec.Generated[bundleConfigFile] = configData

	for i, raw := range spec.VarsFiles {
		dest := fmt.Sprintf("%s/%02d-%s", bundleVarsDir, i, filepath.Base(raw))
		bspec.Files = append(bspec.Files, bundle.File{Source: pathutil.ResolveUserPath(raw), Dest: dest})
		opts.VarsFiles = append(opts.VarsFiles, dest)
	}

	env, err := packages.ParseEnvFile(defaultString(spec.EnvFile, ".env"))
	if err != nil {
		return nil, err
	}
	secretEnv, err := packages.ParseEnvFile(defaultString(spec.SecretsFile, ".secrets"))
	if err != nil {
		return nil, err
	}

	f, err := os.CreateTemp("", "ironstate-bundle-*.tar.gz")
	if err != nil {
		return nil, err
	}
	info, buildErr := bundle.Build(f, bspec)
	closeErr := f.Close()
	if buildErr == nil {
		buildErr = closeErr
	}
	if buildErr != nil {
		_ = os.Remove(f.Name())
		return nil, buildErr
	}

	runID, err := newRunID()
	if err != nil {
		_ = os.Remove(f.Name())
		return nil, err
	}
	for key, value := range spec.ForwardEnv {
		env[key] = value
	}
	prepared.Job = protocol.Job{
		V:                 protocol.Version,
		Type:              protocol.TypeJob,
		RunID:             runID,
		ControllerVersion: spec.ControllerVersion,
		Options:           opts,
		Env:               env,
		SecretEnv:         secretEnv,
		ApprovedUses:      approved,
		Bundle:            protocol.BundleInfo{Size: info.Size, SHA256: info.SHA256},
	}
	prepared.BundlePath = f.Name()
	return prepared, nil
}

// UsesBundleDir is where pre-fetched git 'uses:' checkouts live in the
// bundle; the agent points remote.CacheRoot at it.
const UsesBundleDir = "uses"

// prefetchUses approves (prompting as a local run would) and clones every
// literal git 'uses:' source on the controller, adds each checkout to the
// bundle, and follows 'uses:' nested inside fetched sources. Returns the
// approved source descriptions for the agent's trust check.
func prefetchUses(p *PreparedJob, bspec *bundle.Spec, allowRemote bool) ([]string, error) {
	var approved []string
	seenSource := map[string]bool{}
	bundled := map[string]bool{}
	for i := 0; i < len(p.uses); i++ {
		ref := p.uses[i]
		kind, source := remote.Describe(remote.Spec{Remote: ref.Remote, Path: ref.Path, Ref: ref.Ref})
		if seenSource[source] {
			continue
		}
		seenSource[source] = true
		if remote.NeedsConfirmation(kind, ref.Isolated) && !ref.Trusted && !allowRemote {
			ok, err := remote.Confirm(kind, source)
			if err != nil {
				return nil, err
			}
			if !ok {
				remote.Warn("remote source '%s' was declined; targets will skip it", source)
				continue
			}
		}
		approved = append(approved, source)
		key := remote.CacheKey(strings.TrimSpace(ref.Remote), strings.TrimSpace(ref.Ref))
		if bundled[key] {
			continue
		}
		res, err := remote.Resolve(remote.Spec{Remote: ref.Remote, Ref: ref.Ref}, "")
		if err != nil {
			return nil, fmt.Errorf("pre-fetch uses %s: %w", source, err)
		}
		bundled[key] = true
		bspec.Dirs = append(bspec.Dirs, bundle.Dir{Source: res.Dir, Dest: UsesBundleDir + "/" + key})
		p.PrefetchedUses = append(p.PrefetchedUses, source)
		scanTree(p, res.Dir)
	}
	return approved, nil
}

// scanPlaybook records become, declared plugins, and literal git 'uses:'
// sources. Parse errors are ignored here; the agent reports them properly.
func scanPlaybook(p *PreparedJob, root, entry string) {
	if data, err := os.ReadFile(entry); err == nil { //nolint:gosec // the operator's own playbook
		var doc map[string]any
		if yaml.Unmarshal(data, &doc) == nil {
			p.Plugins, _ = model.Plugins(doc)
		}
	}
	scanTree(p, root)
}

func scanTree(p *PreparedJob, root string) {
	_ = filepath.WalkDir(root, func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(file))
		if ext != ".yml" && ext != ".yaml" {
			return nil
		}
		data, err := os.ReadFile(file) //nolint:gosec // walking the operator's own playbook tree
		if err != nil {
			return nil
		}
		var doc any
		if yaml.Unmarshal(data, &doc) == nil {
			scanValue(p, doc)
		}
		return nil
	})
}

func scanValue(p *PreparedJob, v any) {
	switch val := v.(type) {
	case map[string]any:
		for key, child := range val {
			switch key {
			case "become":
				if becomeRequested(child) {
					p.UsesBecome = true
				}
			case "uses":
				if spec, ok := child.(map[string]any); ok {
					if ref, ok := literalGitUses(spec); ok {
						p.uses = append(p.uses, ref)
					}
				}
			}
			scanValue(p, child)
		}
	case []any:
		for _, child := range val {
			scanValue(p, child)
		}
	}
}

// literalGitUses reads a 'uses:' spec whose remote is a literal git URL; a
// templated remote can't be resolved before the run and isn't pre-fetched.
func literalGitUses(spec map[string]any) (usesRef, bool) {
	src, _ := spec["remote"].(string)
	if strings.Contains(src, "${{") || remote.Classify(src) != remote.KindGit {
		return usesRef{}, false
	}
	ref, _ := spec["ref"].(string)
	sub, _ := spec["path"].(string)
	isolated, _ := spec["isolate"].(bool)
	trusted, _ := spec["trusted"].(bool)
	if strings.Contains(ref, "${{") {
		return usesRef{}, false
	}
	return usesRef{Remote: src, Ref: ref, Path: sub, Isolated: isolated, Trusted: trusted}, true
}

func becomeRequested(v any) bool {
	switch val := v.(type) {
	case nil:
		return false
	case bool:
		return val
	case string:
		trimmed := strings.TrimSpace(val)
		return trimmed != "" && !strings.EqualFold(trimmed, "false")
	default:
		return true
	}
}

// bundleFilters returns the agent's filters.dir setting. A filters dir
// inside the playbook root is already bundled; one outside it is added
// as its own bundle entry next to the playbook.
func bundleFilters(bspec *bundle.Spec, root, configured string) (string, error) {
	configured = defaultString(configured, "filters")
	dir := configured
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(root, dir)
	}
	if rel, err := filepath.Rel(root, dir); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(rel), nil
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return configured, nil
	}
	bspec.Dirs = append(bspec.Dirs, bundle.Dir{Source: dir, Dest: bundleFiltersDir})
	// filters.dir is resolved against the playbook dir on the agent.
	return "../" + bundleFiltersDir, nil
}

func defaultString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func newRunID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
