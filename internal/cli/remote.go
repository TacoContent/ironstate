package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"

	"github.com/TacoContent/ironstate/internal/config"
	"github.com/TacoContent/ironstate/internal/engine"
	"github.com/TacoContent/ironstate/internal/remoteexec"
	"github.com/TacoContent/ironstate/internal/remoteexec/protocol"
	"github.com/TacoContent/ironstate/internal/secrets"
	"github.com/TacoContent/ironstate/internal/ui"
)

// addRemoteFlags registers the flags shared by remote apply and 'remote ping'.
func addRemoteFlags(flags *pflag.FlagSet) {
	flags.StringArray("target", nil, "apply to a remote host over SSH ([user@]host[:port] or an ssh config alias; 'local' runs the agent locally); repeatable")
	flags.String("inventory", "", "inventory file of remote hosts and groups (see inventory.schema.json)")
	flags.StringSlice("limit", nil, "with --inventory: only these hosts/groups ('all' = every host); comma-separated or repeatable")
	flags.Int("forks", 5, "how many hosts to run at once")
	flags.Duration("host-timeout", 0, "maximum duration for one remote host operation (e.g. 5m; 0 disables)")
	flags.StringArray("agent-binary", nil, "ironstate binary to ship for a target platform: os/arch=path (e.g. linux/arm64=./dist/ironstate); repeatable")
	flags.Bool("no-agent-download", false, "never download a release agent binary for a target platform; use --agent-binary or the local agent cache")
	flags.String("remote-agent-dir", "", "directory on targets for the cached agent binary (default: $XDG_CACHE_HOME or ~/.cache /ironstate/agent)")
	flags.String("ssh-config", "", "ssh config file passed to ssh -F")
	flags.Bool("ssh-accept-new-host-keys", false, "accept and remember host keys of never-seen targets (StrictHostKeyChecking=accept-new)")
	flags.Bool("skip-agent-verification", false, "skip target-side SHA-256 verification for trusted targets without a hash utility; always reuploads the agent")
}

// remoteRequested reports whether flags select remote hosts at all.
func remoteRequested(flags *pflag.FlagSet) bool {
	targets, _ := flags.GetStringArray("target")
	inventory, _ := flags.GetString("inventory")
	return len(targets) > 0 || inventory != ""
}

// resolveHosts returns the inventory selection followed by any --target
// hosts, without duplicate names.
func resolveHosts(flags *pflag.FlagSet) ([]remoteexec.Host, error) {
	inventoryPath, _ := flags.GetString("inventory")
	limit, _ := flags.GetStringSlice("limit")
	targets, _ := flags.GetStringArray("target")
	var hosts []remoteexec.Host
	if inventoryPath != "" {
		inv, err := remoteexec.LoadInventory(inventoryPath)
		if err != nil {
			return nil, err
		}
		if hosts, err = inv.Select(limit); err != nil {
			return nil, err
		}
	} else if len(limit) > 0 {
		return nil, fmt.Errorf("--limit needs --inventory")
	}
	seen := map[string]bool{}
	for _, h := range hosts {
		seen[h.Name] = true
	}
	for _, target := range targets {
		host, err := remoteexec.HostFromTarget(target)
		if err != nil {
			return nil, err
		}
		if !seen[host.Name] {
			seen[host.Name] = true
			hosts = append(hosts, host)
		}
	}
	if len(hosts) == 0 {
		return nil, fmt.Errorf("no hosts selected")
	}
	return hosts, nil
}

func remoteHostOptions(flags *pflag.FlagSet) (remoteexec.HostOptions, error) {
	var opts remoteexec.HostOptions
	binaries := map[string]string{}
	specs, _ := flags.GetStringArray("agent-binary")
	for _, spec := range specs {
		platform, file, ok := strings.Cut(spec, "=")
		if !ok || !strings.Contains(platform, "/") || file == "" {
			return opts, fmt.Errorf("--agent-binary %q: want os/arch=path", spec)
		}
		binaries[platform] = file
	}
	noDownload, _ := flags.GetBool("no-agent-download")
	opts.Agents = &remoteexec.AgentSource{Binaries: binaries, Version: version, NoDownload: noDownload}
	opts.AgentDir, _ = flags.GetString("remote-agent-dir")
	opts.HostTimeout, _ = flags.GetDuration("host-timeout")
	if opts.HostTimeout < 0 {
		return opts, errors.New("--host-timeout must be zero or a positive duration")
	}
	opts.Detach, _ = flags.GetBool("remote-detach")
	opts.SkipAgentVerification, _ = flags.GetBool("skip-agent-verification")
	opts.SSH.ConfigFile, _ = flags.GetString("ssh-config")
	opts.SSH.AcceptNewHostKeys, _ = flags.GetBool("ssh-accept-new-host-keys")
	return opts, nil
}

// remoteSignals turns the first Ctrl-C into a graceful cancel (agents stop
// after their current task) and the second into an abort.
func remoteSignals(parent context.Context, stderr io.Writer) (context.Context, <-chan struct{}, func()) {
	ctx, abort := context.WithCancel(parent)
	cancelCh := make(chan struct{})
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		count := 0
		for range sigCh {
			count++
			if count == 1 {
				_, _ = fmt.Fprintln(stderr, ui.Yellow("⚠ stopping after the current task on each host; press Ctrl-C again to abort"))
				close(cancelCh)
				continue
			}
			abort()
			return
		}
	}()
	return ctx, cancelCh, func() {
		signal.Stop(sigCh)
		abort()
	}
}

func interruptContext(parent context.Context, interrupts <-chan os.Signal) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	stopped := make(chan struct{})
	go func() {
		select {
		case <-interrupts:
			cancel()
		case <-ctx.Done():
		case <-stopped:
		}
	}()
	return ctx, func() {
		close(stopped)
		cancel()
	}
}

func operationContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, timeout)
}

func runRemoteApply(cmd *cobra.Command, cfg *config.Config) error {
	hosts, err := resolveHosts(cmd.Flags())
	if err != nil {
		return NewLoadError(err)
	}
	opts, err := remoteHostOptions(cmd.Flags())
	if err != nil {
		return NewLoadError(err)
	}
	forks, _ := cmd.Flags().GetInt("forks")
	view, err := newRemoteView(cmd, cfg.Output)
	if err != nil {
		return NewLoadError(err)
	}
	forward, err := forwardedEnv(cmd.Flags())
	if err != nil {
		return NewLoadError(err)
	}
	job, err := remoteexec.Prepare(remoteexec.JobSpec{
		Playbook:           cfg.Playbook,
		VarsFiles:          cfg.VarsFiles,
		VarOverrides:       cfg.VarOverrides,
		Tags:               cfg.Tags,
		Apply:              cfg.Apply,
		Verbose:            cfg.Verbose,
		FiltersDir:         cfg.FiltersDir,
		FilterInterpreters: cfg.FilterInterpreters,
		ControllerVersion:  version,
		ForwardEnv:         forward,
	})
	if err != nil {
		return NewLoadError(err)
	}
	defer func() { _ = job.Close() }()
	if opts.Detach && !job.Job.Options.Apply {
		return NewLoadError(errors.New("--remote-detach requires --apply"))
	}
	if opts.Detach && (len(job.Job.Env) > 0 || len(job.Job.SecretEnv) > 0) {
		return NewLoadError(errors.New("--remote-detach cannot stage .env, .secrets, or forwarded environment on the target"))
	}
	askBecomePass, _ := cmd.Flags().GetBool("ask-become-pass")
	if opts.Detach && askBecomePass && job.UsesBecome {
		return NewLoadError(errors.New("--remote-detach cannot stage a become password on the target"))
	}
	if askBecomePass && job.UsesBecome {
		password, err := promptBecomePassword(cmd)
		if err != nil {
			return NewLoadError(err)
		}
		job.Job.BecomePassword = password
		secrets.Register(password)
	}

	ctx, cancelCh, stopSignals := remoteSignals(cmd.Context(), cmd.ErrOrStderr())
	defer stopSignals()
	opts.Cancel = cancelCh

	reports := remoteexec.ForEachHost(hosts, forks, func(host remoteexec.Host) remoteexec.HostReport {
		if remoteexec.Cancelled(cancelCh) {
			report := remoteexec.NotStarted(host)
			view.hostDone(report)
			return report
		}
		hostOpts := opts
		hostOpts.OnEvent = func(ev protocol.Event) { view.event(host.Name, ev) }
		view.hostStart(host)
		report := remoteexec.ApplyHost(ctx, host, job, hostOpts)
		view.hostDone(report)
		return report
	})

	code := remoteexec.ExitCode(reports)
	if err := view.finish(job.Job.RunID, reports, code); err != nil {
		return NewRunError(err)
	}
	if code != 0 {
		return &ExitCodeError{Code: code, Err: fmt.Errorf("remote apply: %s", summarizeStatuses(reports))}
	}
	return nil
}

func promptBecomePassword(cmd *cobra.Command) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("--ask-become-pass requires an interactive terminal")
	}
	if _, err := fmt.Fprint(cmd.ErrOrStderr(), "Become password: "); err != nil {
		return "", err
	}
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	_, _ = fmt.Fprintln(cmd.ErrOrStderr())
	if err != nil {
		return "", fmt.Errorf("read become password: %w", err)
	}
	return string(password), nil
}

func forwardedEnv(flags *pflag.FlagSet) (map[string]string, error) {
	names, _ := flags.GetStringArray("forward-env")
	env := map[string]string{}
	for _, name := range names {
		value, ok := os.LookupEnv(name)
		if !ok {
			return nil, fmt.Errorf("--forward-env %s: not set in the controller environment", name)
		}
		env[name] = value
	}
	return env, nil
}

func summarizeStatuses(reports []remoteexec.HostReport) string {
	counts := map[string]int{}
	for _, r := range reports {
		counts[r.Status]++
	}
	var parts []string
	for _, status := range []string{remoteexec.StatusFailed, remoteexec.StatusUnreachable, remoteexec.StatusError, remoteexec.StatusOK} {
		if counts[status] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[status], status))
		}
	}
	return strings.Join(parts, ", ")
}

// sanitizeRemote strips terminal control sequences from remote-origin text.
func sanitizeRemote(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || !unicode.IsControl(r) {
			return r
		}
		return -1
	}, s)
}

type remoteView struct {
	format string
	out    io.Writer
	errOut io.Writer
	events *protocol.Writer
	// mu keeps concurrent hosts' live lines whole.
	mu sync.Mutex
}

func newRemoteView(cmd *cobra.Command, format string) (*remoteView, error) {
	switch format {
	case "", "table", "json", "ndjson":
	default:
		return nil, fmt.Errorf("unknown --output %q (want table, json or ndjson)", format)
	}
	v := &remoteView{format: format, out: cmd.OutOrStdout(), errOut: cmd.ErrOrStderr()}
	if format == "ndjson" {
		v.events = protocol.NewWriter(v.out)
	}
	return v, nil
}

func (v *remoteView) hostStart(host remoteexec.Host) {
	if v.events != nil {
		return
	}
	line := "▶ " + host.Name
	if addr := host.Address(); addr != host.Name {
		line += " (" + addr + ")"
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	_, _ = fmt.Fprintln(v.errOut, ui.Bold(line))
}

func (v *remoteView) event(target string, ev protocol.Event) {
	if v.events != nil {
		ev.Host = target
		_ = v.events.Emit(ev)
		return
	}
	if ev.Type != protocol.TypeLog {
		return
	}
	msg := fmt.Sprintf("[%s] %s", target, sanitizeRemote(ev.Message))
	switch ev.Level {
	case protocol.LevelWarn:
		msg = ui.Yellow("⚠ " + msg)
	case protocol.LevelDanger:
		msg = ui.BoldRed("✖ " + msg)
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	_, _ = fmt.Fprintln(v.errOut, msg)
}

func (v *remoteView) hostDone(r remoteexec.HostReport) {
	if v.events != nil {
		if r.Detached {
			runID := ""
			if r.Result != nil {
				runID = r.Result.RunID
			}
			_ = v.events.Emit(protocol.Event{Type: protocol.TypeLog, Host: r.Name, Level: protocol.LevelInfo, Message: "detached; run_id=" + runID})
			return
		}
		if r.Result == nil || !r.Result.Completed {
			code := 3
			_ = v.events.Emit(protocol.Event{Type: protocol.TypeError, Host: r.Name, Phase: "connect", Error: errString(r.Err)})
			_ = v.events.Emit(protocol.Event{Type: protocol.TypeDone, Host: r.Name, ExitCode: &code})
		}
		return
	}
	line := fmt.Sprintf("%s %s (%s)", statusLabel(r.Status), r.Name, r.Duration.Round(time.Millisecond))
	if r.Detached && r.Result != nil {
		line += " detached run=" + r.Result.RunID
	}
	if r.VerificationSkipped {
		line += " agent-verification=skipped"
	}
	if r.Err != nil {
		line += ": " + sanitizeRemote(r.Err.Error())
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	_, _ = fmt.Fprintln(v.errOut, line)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func statusLabel(status string) string {
	switch status {
	case remoteexec.StatusOK:
		return ui.BoldGreen("✔ ok")
	case remoteexec.StatusFailed:
		return ui.BoldRed("✖ failed")
	case remoteexec.StatusUnreachable:
		return ui.BoldYellow("⚠ unreachable")
	default:
		return ui.BoldRed("✖ error")
	}
}

func (v *remoteView) finish(runID string, reports []remoteexec.HostReport, code int) error {
	switch v.format {
	case "ndjson":
		return v.events.Done(code)
	case "json":
		return writeRemoteJSON(v.out, runID, reports)
	default:
		return v.writeTable(reports)
	}
}

func hostComputerName(r remoteexec.HostReport) string {
	if r.Result == nil {
		return ""
	}
	name, _ := r.Result.Facts["computer_name"].(string)
	return sanitizeRemote(name)
}

func hostStats(r remoteexec.HostReport) engine.Stats {
	if r.Result == nil {
		return engine.Stats{}
	}
	if r.Result.Summary != nil && r.Result.Summary.Stats != nil {
		return *r.Result.Summary.Stats
	}
	// A run aborted by an error has no summary; count what streamed in.
	return engine.ComputeStats(resultsFromJSON(r.Result.Results))
}

func (v *remoteView) writeTable(reports []remoteexec.HostReport) error {
	for _, r := range reports {
		if r.Result == nil || len(r.Result.Results) == 0 {
			continue
		}
		title := r.Name
		if cn := hostComputerName(r); cn != "" && !strings.EqualFold(cn, r.Name) {
			title += " (" + cn + ")"
		}
		if err := ui.WriteLine(v.out, ui.Bold("── "+title+" ──")); err != nil {
			return err
		}
		if err := engine.PrintTable(v.out, resultsFromJSON(r.Result.Results)); err != nil {
			return err
		}
		if err := ui.WriteLine(v.out, ""); err != nil {
			return err
		}
	}

	header := []string{"HOST", "STATUS", "TOTAL", "INSTALLED", "UNINSTALLED", "SKIPPED", "FAILED", "TIME"}
	rows := make([][]string, 0, len(reports))
	for _, r := range reports {
		s := hostStats(r)
		rows = append(rows, []string{r.Name, r.Status, fmt.Sprint(s.Total), fmt.Sprint(s.Installed), fmt.Sprint(s.Uninstalled), fmt.Sprint(s.Skipped), fmt.Sprint(s.Failed), r.Duration.Round(time.Millisecond).String()})
	}
	widths := make([]int, len(header))
	for i, h := range header {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			widths[i] = max(widths[i], len(cell))
		}
	}
	render := func(cells []string, statusColor func(string) string) string {
		parts := make([]string, len(cells))
		for i, cell := range cells {
			padded := fmt.Sprintf("%-*s", widths[i], cell)
			if i == 1 && statusColor != nil {
				padded = statusColor(padded)
			}
			parts[i] = padded
		}
		return strings.Join(parts, "  ")
	}
	if err := ui.WriteLine(v.out, ui.Bold(render(header, nil))); err != nil {
		return err
	}
	for i, row := range rows {
		if err := ui.WriteLine(v.out, render(row, statusColor(reports[i].Status))); err != nil {
			return err
		}
	}
	return nil
}

func statusColor(status string) func(string) string {
	switch status {
	case remoteexec.StatusOK:
		return ui.BoldGreen
	case remoteexec.StatusUnreachable:
		return ui.BoldYellow
	default:
		return ui.BoldRed
	}
}

func resultsFromJSON(in []engine.JSONResult) []engine.Result {
	out := make([]engine.Result, len(in))
	for i, r := range in {
		out[i] = engine.Result{
			Module:   sanitizeRemote(r.Module),
			Package:  sanitizeRemote(r.Package),
			State:    sanitizeRemote(r.State),
			Action:   r.Action,
			Apply:    r.Apply,
			Failed:   r.Failed,
			Duration: time.Duration(r.DurationMS * float64(time.Millisecond)),
		}
	}
	return out
}

type remoteJSONHost struct {
	Name                string              `json:"name"`
	Address             string              `json:"address,omitempty"`
	ComputerName        string              `json:"computer_name,omitempty"`
	Platform            string              `json:"platform,omitempty"`
	Status              string              `json:"status"`
	ExitCode            int                 `json:"exit_code"`
	Facts               map[string]any      `json:"facts,omitempty"`
	Results             []engine.JSONResult `json:"results"`
	Stats               engine.Stats        `json:"stats"`
	DurationMS          float64             `json:"duration_ms"`
	Uploaded            bool                `json:"agent_uploaded"`
	Detached            bool                `json:"detached"`
	VerificationSkipped bool                `json:"agent_verification_skipped"`
	Error               *string             `json:"error"`
}

type remoteJSONStats struct {
	Hosts       int `json:"hosts"`
	OK          int `json:"ok"`
	Failed      int `json:"failed"`
	Unreachable int `json:"unreachable"`
	Error       int `json:"error"`
}

func writeRemoteJSON(w io.Writer, runID string, reports []remoteexec.HostReport) error {
	doc := struct {
		RunID string           `json:"run_id"`
		Hosts []remoteJSONHost `json:"hosts"`
		Stats remoteJSONStats  `json:"stats"`
	}{RunID: runID, Hosts: []remoteJSONHost{}}
	for _, r := range reports {
		h := remoteJSONHost{
			Name:                r.Name,
			Address:             r.Address,
			ComputerName:        hostComputerName(r),
			Platform:            r.Platform,
			Status:              r.Status,
			Stats:               hostStats(r),
			DurationMS:          float64(r.Duration) / float64(time.Millisecond),
			Uploaded:            r.Uploaded,
			Detached:            r.Detached,
			VerificationSkipped: r.VerificationSkipped,
			Results:             []engine.JSONResult{},
		}
		if r.Result != nil {
			h.ExitCode = r.Result.ExitCode
			h.Facts = r.Result.Facts
			if r.Result.Results != nil {
				h.Results = r.Result.Results
			}
		}
		if r.Err != nil {
			msg := r.Err.Error()
			h.Error = &msg
		}
		doc.Hosts = append(doc.Hosts, h)
		doc.Stats.Hosts++
		switch r.Status {
		case remoteexec.StatusOK:
			doc.Stats.OK++
		case remoteexec.StatusFailed:
			doc.Stats.Failed++
		case remoteexec.StatusUnreachable:
			doc.Stats.Unreachable++
		default:
			doc.Stats.Error++
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

func newRemoteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remote",
		Short: "Remote apply helpers",
	}
	ping := &cobra.Command{
		Use:   "ping",
		Short: "Connect to targets, probe them, and make sure the agent runs (applies nothing)",
		Args:  cobra.NoArgs,
		RunE:  runRemotePing,
	}
	addRemoteFlags(ping.Flags())
	cmd.AddCommand(ping)

	logs := &cobra.Command{
		Use:   "logs <host> [run_id]",
		Short: "Read a target's latest or selected remote apply event log",
		Args:  cobra.RangeArgs(1, 2),
		RunE:  runRemoteLogs,
	}
	addRemoteFlags(logs.Flags())
	cmd.AddCommand(logs)

	clean := &cobra.Command{
		Use:   "clean",
		Short: "Remove cached agents and retained run logs on selected targets",
		Args:  cobra.NoArgs,
		RunE:  runRemoteClean,
	}
	addRemoteFlags(clean.Flags())
	cmd.AddCommand(clean)
	return cmd
}

func managementHost(flags *pflag.FlagSet, name string) (remoteexec.Host, error) {
	inventoryPath, _ := flags.GetString("inventory")
	if inventoryPath == "" {
		return remoteexec.HostFromTarget(name)
	}
	inv, err := remoteexec.LoadInventory(inventoryPath)
	if err != nil {
		return remoteexec.Host{}, err
	}
	hosts, err := inv.Select([]string{name})
	if err != nil {
		return remoteexec.Host{}, err
	}
	if len(hosts) != 1 {
		return remoteexec.Host{}, fmt.Errorf("host %q selects %d hosts; remote logs requires one host", name, len(hosts))
	}
	return hosts[0], nil
}

func runRemoteLogs(cmd *cobra.Command, args []string) error {
	host, err := managementHost(cmd.Flags(), args[0])
	if err != nil {
		return NewLoadError(err)
	}
	opts, err := remoteHostOptions(cmd.Flags())
	if err != nil {
		return NewLoadError(err)
	}
	ctx, cancel := operationContext(cmd.Context(), opts.HostTimeout)
	defer cancel()
	runID := ""
	if len(args) == 2 {
		runID = args[1]
	}
	result, err := remoteexec.ReadHostLogs(ctx, host, opts, runID)
	if err != nil {
		return NewRunError(err)
	}
	if _, err := io.WriteString(cmd.OutOrStdout(), result.RawLog); err != nil {
		return NewRunError(err)
	}
	return nil
}

func runRemoteClean(cmd *cobra.Command, _ []string) error {
	hosts, err := resolveHosts(cmd.Flags())
	if err != nil {
		return NewLoadError(err)
	}
	opts, err := remoteHostOptions(cmd.Flags())
	if err != nil {
		return NewLoadError(err)
	}
	forks, _ := cmd.Flags().GetInt("forks")
	reports := remoteexec.ForEachHost(hosts, forks, func(host remoteexec.Host) remoteexec.HostReport {
		started := time.Now()
		ctx, cancel := operationContext(cmd.Context(), opts.HostTimeout)
		defer cancel()
		err := remoteexec.CleanHost(ctx, host, opts)
		report := remoteexec.HostReport{Name: host.Name, Address: host.Address(), Duration: time.Since(started)}
		if err != nil {
			report.Status, report.Err = remoteexec.StatusError, err
		} else {
			report.Status = remoteexec.StatusOK
		}
		return report
	})
	for _, report := range reports {
		line := fmt.Sprintf("%s %s", statusLabel(report.Status), report.Name)
		if report.Err != nil {
			line += ": " + sanitizeRemote(report.Err.Error())
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
	}
	if code := remoteexec.ExitCode(reports); code != 0 {
		return &ExitCodeError{Code: code, Err: fmt.Errorf("remote clean: %s", summarizeStatuses(reports))}
	}
	return nil
}

func runRemotePing(cmd *cobra.Command, _ []string) error {
	hosts, err := resolveHosts(cmd.Flags())
	if err != nil {
		return NewLoadError(err)
	}
	opts, err := remoteHostOptions(cmd.Flags())
	if err != nil {
		return NewLoadError(err)
	}
	forks, _ := cmd.Flags().GetInt("forks")
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	ctx, stop := interruptContext(cmd.Context(), sigCh)
	defer func() {
		signal.Stop(sigCh)
		stop()
	}()
	var mu sync.Mutex
	reports := remoteexec.ForEachHost(hosts, forks, func(host remoteexec.Host) remoteexec.HostReport {
		r := remoteexec.PingHost(ctx, host, opts)
		line := fmt.Sprintf("%s %s", statusLabel(r.Status), host.Name)
		if host.Name != r.Address {
			line += " (" + r.Address + ")"
		}
		if r.Platform != "" {
			line += "  " + r.Platform
		}
		if r.Probe.Sudo != "" {
			line += "  sudo=" + r.Probe.Sudo
		}
		if r.VerificationSkipped {
			line += "  agent-verification=skipped"
		}
		if r.Status == remoteexec.StatusOK {
			line += "  agent: " + sanitizeRemote(r.AgentVersion)
			if r.Uploaded {
				line += " (uploaded)"
			}
		} else if r.Err != nil {
			line += ": " + sanitizeRemote(r.Err.Error())
		}
		mu.Lock()
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
		mu.Unlock()
		return r.HostReport
	})
	if ctx.Err() != nil {
		return &ExitCodeError{Code: 130, Err: fmt.Errorf("remote ping interrupted: %w", ctx.Err())}
	}
	if code := remoteexec.ExitCode(reports); code != 0 {
		return &ExitCodeError{Code: code, Err: fmt.Errorf("remote ping: %s", summarizeStatuses(reports))}
	}
	return nil
}
