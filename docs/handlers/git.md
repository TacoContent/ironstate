# git

Modeled on Ansible's git module, with a focused option set for ironstate. Uses the local `git` CLI and supports clone/update/checkout/submodule reconciliation.

## Supported fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `repo` | yes | - | Repository URL/path used by `git clone` |
| `dest` | yes | - | Checkout destination path (`~` expansion supported) |
| `ref`  | no | `HEAD` | Branch/tag/commit to check out |
| `update` | no | `true` | Fetch/pull existing checkout to reconcile latest state |
| `clone` | no | `true` | If `false`, missing `dest` is an error instead of cloning |
| `force` | no | `false` | Replaces non-git or remote-mismatched `dest` checkouts |
| `depth` | no | unset | Shallow clone/fetch depth |
| `single_branch` | no | `false` | Use `--single-branch` during clone |
| `recursive` | no | `true` | Run `git submodule update --init --recursive` after checkout |
| `state` | no | `present` | Standard `present` / `absent` / `latest` state machine |

## State behavior

### state: present

- If `dest` is missing: clone repository (unless `clone: false`, which errors).
- If `dest` exists and is not a git worktree:
  - `force: false` => error
  - `force: true` => remove and re-clone
- If `dest` points at a different `remote.origin.url` than `repo`:
  - `force: false` => error
  - `force: true` => remove and re-clone
- If `update: true`, fetches before checkout.
- If `ref != HEAD`, checks out that ref.
- If `ref == HEAD` and `update: true`, runs `git pull --ff-only`.
- If `recursive: true`, updates submodules recursively.

### state: latest

- Behaves like `present`, but is intentionally treated as not-converged when `update: true`, so update steps run each time.

### state: absent

- Removes `dest` recursively when it exists.

## Notes

- This handler intentionally uses a smaller option surface than Ansible's full git module while keeping similar operational semantics for common workflows.
- The `git` executable must be on `PATH`.

## Examples

### Clone a repository

```yaml
tasks:
  - name: Clone dotfiles
    git:
      repo: https://github.com/example/dotfiles.git
      dest: ~/.dotfiles
```

### Pin a tag

```yaml
tasks:
  - name: Checkout v1.2.3
    git:
      repo: https://github.com/example/tool.git
      dest: ~/.src/tool
      ref: v1.2.3
```

### Track latest default branch with shallow history

```yaml
tasks:
  - name: Keep checkout current
    git:
      repo: https://github.com/example/project.git
      dest: ~/.src/project
      state: latest
      depth: 1
      single_branch: true
```

### Replace mismatched checkout

```yaml
tasks:
  - name: Replace wrong origin checkout
    git:
      repo: git@github.com:example/project.git
      dest: ~/.src/project
      force: true
```

### Remove checkout

```yaml
tasks:
  - name: Remove old checkout
    git:
      dest: ~/.src/old-project
      repo: https://github.com/example/old-project.git
      state: absent
```
