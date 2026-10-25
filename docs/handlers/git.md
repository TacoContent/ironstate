---
layout: default
title: git
parent: 👋 Handlers
nav_order: 19
---

# git

The `git` handler manages a git checkout using the local `git` CLI, modeled on Ansible's `git` module with a focused option set.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `repo` | yes | - | Repository URL/path used by `git clone` |
| `dest` | yes | - | Checkout destination path (`~` expansion supported) |
| `ref` / `version` | no | `HEAD` | Branch/tag/commit to check out |
| `update` | no | `true` | Fetch/pull existing checkout to reconcile latest state |
| `clone` | no | `true` | If `false`, missing `dest` is an error instead of cloning |
| `force` | no | `false` | Replaces non-git or remote-mismatched `dest` checkouts |
| `depth` | no | - | Shallow clone/fetch depth |
| `single_branch` | no | `false` | Use `--single-branch` during clone |
| `recursive` | no | `true` | Run `git submodule update --init --recursive` after checkout |
| `state` | no | `present` | `present` / `absent` / `latest` |

## State behavior

- `state: present` - Clone if missing (unless `clone: false`, errors). If dest exists but not a git worktree: error unless `force: true` (removes and re-clones). If remote.origin.url differs: error unless `force: true`. If `update: true`, fetch before checkout. If `ref != HEAD`, checkout that ref. If `ref == HEAD` and `update: true`, runs `git pull --ff-only`. If `recursive: true`, updates submodules recursively.
- `state: latest` - Behaves like `present`, but treated as not-converged when `update: true` so update steps run each time.
- `state: absent` - Removes `dest` recursively when it exists.

## Notes

- The `git` executable must be on `PATH`.
- Handler uses markers/logic similar to Ansible for idempotency.
