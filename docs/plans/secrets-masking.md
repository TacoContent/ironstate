# Secrets / Sensitive-Value Masking — Design Plan

Status: PROPOSED (not yet implemented)

## Problem

`ironstate` currently has no way to mark a fact, `vars:` entry, or `id`-registered task
result as sensitive. Any of these can end up printed verbatim - the facts panel, a
`fact` task's own "set fact 'X' = value" line, a `log` message, a rendered
`shell`/`template` field, the results table, `--output json`, etc. - with no way to
suppress it. A secret sourced from `.env`/`.secrets` (see [README's `.env`/`.secrets`
section](../../README.md#env--secrets)) or a `lookup('env', ...)` call is especially at
risk here, since it's specifically meant to be sensitive. `.secrets` in particular is
named and documented as the sensitive-value file (as opposed to `.env`, for ordinary
config) - **anything loaded from it should be masked automatically**, with no `$`
authoring needed at all (see Phase 0 below); the `$`-prefix convention (Phases 1-3)
covers everything *else* that's sensitive but doesn't happen to live in `.secrets`.

## Marking convention

**A name whose first character is `$` is a secret**, applied uniformly across facts,
`vars:`, and `id`:

```yaml
vars:
  $my_variable: 'abc123'   # referenced as vars.my_variable (bare, no '$') - flagged secret

tasks:
  - id: $api_token          # referenced as api_token.rc / api_token.stdout - flagged secret
    shell:
      command: Get-Secret

  - fact:
      name: $github_token   # referenced as facts.github_token - flagged secret
      shell:
        command: gh auth token
```

The `$` is stripped for the actual registered/accessible name; it only ever exists as a
marker on the *authored* key. This is a single, uniform mechanism rather than a
per-module `secret: true` field (the alternative raised) - one convention to document,
one thing for a reader of someone else's `main.yml` to recognize, and it works
identically whether the sensitive value comes from a fact, a var, or a registered task
result, without a per-module opt-in field to remember to add everywhere.

## Design: redact known secret *values*, not just known secret *names*

Trying to special-case every place a fact/var/id's value could end up displayed (facts
panel, every `Describe()` string, every module's own log/print, the results table,
JSON output, a `log` message that embeds `${{ }}`, ...) is a losing battle - the
surface area is every module and every future one. Instead, model this the way CI
systems mask secrets in logs (GitHub Actions, etc.): once a secret's *value* is known,
register that exact string in a small runtime registry, and redact any later
occurrence of it from anything `ironstate` itself prints.

```go
// internal/secrets (new package)
func Register(value string)      // no-op below a minimum length (see Limitations)
func Redact(text string) string  // replaces every registered value with "***"
```

- `internal/engine.Info`/`Warn`/`Danger` run their formatted message through
  `secrets.Redact` before printing.
- `internal/engine.PrintTable`/`PrintJSON` redact every string field (`Package`,
  `State`, `Exec.Stdout`/`Stderr`/`*Lines`) the same way - `--output json` must not
  become the "safe" channel to leak a secret through.
- `internal/ui.PrintFacts` redacts fact values the same way (relevant once/if it ever
  shows accumulated user-defined facts, not just the initial gathered set).

A secret's value is registered **the moment it becomes known** - e.g. right after a
`fact: { name: $token, shell: {...} }` task's command produces output, before its
`Describe()`/`Info()` line prints anything referencing it - not lazily on first print.

## Phased rollout (`.secrets` first - automatic, no authoring - then facts, then vars, then `id`)

### Phase 0 — anything loaded from `.secrets` (automatic, no `$` needed)

Unlike Phases 1-3, this needs no marking convention at all: **every value loaded from
`.secrets` is a secret by definition** - that's the whole reason it's a separate file
from `.env`. This is also the simplest phase to land first, since it hooks in at exactly
one place (env-file loading, already fixed to actually run - see the progress notes)
rather than needing changes spread across facts/vars/`id`.

- `internal/packages/env.go`'s `ImportEnvFile` currently returns only an `error`. Add a
  variant (or change the signature) so the caller gets back the `KEY -> VALUE` pairs it
  loaded, e.g. `ImportEnvFile(path string) (map[string]string, error)`.
- `internal/cli/root.go`'s `runApply`: keep loading `.env` as today (not auto-secret -
  it's for ordinary config), but for `.secrets` specifically, take the returned map and
  call `secrets.Register(value)` for every loaded value, immediately after the file is
  read - before `packages.LoadHierarchy`, facts gathering, or any dispatch happens, so
  there's no window where a `.secrets` value could print unmasked.
- This composes for free with `lookup('env', 'SOME_SECRETS_KEY')`: the *value* returned
  is already a registered secret by the time anything tries to print it, regardless of
  which fact/var/task field it flows into - no `$`-marking needed on the consuming side
  at all.
- New tests: a value loaded from a temp `.secrets` file is redacted from `Info`/`Warn`
  output and from `PrintTable`/`PrintJSON` when a task's `log.message`/`Exec.Stdout`
  echoes it (e.g. via `lookup('env', ...)`); the equivalent value loaded from `.env`
  is **not** auto-redacted (proves the two files are treated differently, as intended).

### Phase 1 — Facts
- `internal/handlers/fact.go`: if `item["name"]` starts with `$`, strip the prefix for
  the actual `state.UserFacts` key, and record the bare name in a new
  `engine.State.SecretFacts map[string]bool` set.
- `fact.go`'s `Describe()` (currently something like `"set fact '%s' = %v"`) checks
  `SecretFacts[name]` and substitutes `***` instead of the real value when secret.
- Whenever a secret fact's value is computed (embedded-shell case or plain `value:`),
  call `secrets.Register(value)` immediately after it's known.
- New tests: a `$`-prefixed fact's `Describe()` output never contains the real value;
  `facts.token` still resolves to the real value in a later `when`/`${{ }}` (masking is
  print-only, never functional).

### Phase 2 — `vars:`
- After `vars := model.Vars(docMap)` in `internal/cli/root.go`, run one recursive pass
  over the merged `vars` map: for every key at any depth starting with `$`, strip the
  prefix (so the tree ends up with only the bare, accessible names) and record its full
  dotted path (e.g. `github.token`) in a `secretVarPaths map[string]bool` set.
- Whenever `internal/expr.Eval` resolves a `PathNode` whose path matches an entry in
  `secretVarPaths` to a concrete (non-nil) value, call `secrets.Register` on it before
  returning - this is the one chokepoint every `${{ vars.* }}`/bare `when` reference
  already goes through, so it covers every consumer (task fields, `log` messages,
  `shell.command`, template render contexts, ...) without touching each module.
- New tests: a `$`-prefixed var's value never appears in `PrintTable`/`PrintJSON`/`Info`
  output even when referenced from an arbitrary field (e.g. a `log.message`); the value
  is still usable normally in `when`/filters/comparisons.

### Phase 3 — `id`
- `internal/tasks`: if a leaf's authored `id` starts with `$`, strip the prefix for
  `Leaf.ID` (registry key) and set a new `Leaf.SecretID bool`.
- `internal/engine.registerLeafResult`: when `leaf.SecretID`, call `secrets.Register` on
  every string value being stored (`rc` as text, `stdout`, `stdout_lines`, `stderr`,
  `stderr_lines`, and any merged native `Extra` string properties) before/as it's
  written into `state.Registry`.
- New tests: a `$`-prefixed `id`'s captured `stdout` never appears in output, but
  `foo.rc != 0`-style `when` conditions referencing it still work.

## Open questions / follow-ups (not blocking Phase 0/1)

- **Minimum length before registering a redaction value.** An unguarded `secrets.Register`
  would redact trivially common short strings (e.g. a token that happens to equal `"1"`
  or `"true"`) and mangle unrelated output. Recommend a floor (e.g. 6 characters) below
  which a value is never auto-redacted - document this limitation plainly rather than
  pretend short secrets are protected.
- **Filtered/transformed secrets aren't caught.** `${{ vars.token | upper }}` or
  `${{ vars.token | sha1 }}` produces a *different* string than the registered value, so
  it won't be redacted. This is the same limitation CI secret-masking has; call it out
  in the README once implemented rather than silently under-protecting.
- **`--output json` redaction changes the contract**: today's JSON output is meant to be
  exact/parseable; substituting `***` into `Exec.Stdout` for a secret leaf is a
  deliberate, intentional break from "exact" in favor of "safe by default" - worth a
  one-line callout in the README's `--output json` description once shipped.
- Not in scope for this plan: encrypting secrets at rest (e.g. an Ansible-Vault-style
  feature) - this is display-time masking only, matching `.env`/`.secrets`'s existing
  plaintext-on-disk trust model.
- **A `.secrets` value re-authored into `vars:`/a fact under a *non*-`$`-prefixed name**
  (e.g. `vars: { token: ${{ lookup('env', 'MY_SECRETS_KEY') }} }`) is still masked under
  Phase 0's value-based registration (the value itself is already known-secret,
  regardless of what name it's later assigned to) - Phase 0 and Phases 1-3 are
  complementary, not redundant: Phase 0 catches secrets by where they *came from*,
  Phases 1-3 catch secrets by how they're *authored*, and either one registering a value
  is enough to redact it everywhere.
