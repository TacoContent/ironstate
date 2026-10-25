---
render_with_liquid: false
layout: default
title: 🧹 Filters
nav_order: 6
has_children: true
---
<!-- markdownlint-disable-next-line MD025 MD022 -->
# 🧹 Filters
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
Filters transform a value inside an expression. They work like Jinja or Ansible filters: you pipe a value into a named filter with `|`, optionally pass arguments, and chain as many as you like from left to right.

```yaml
vars:
  editor: "  NVim "

tasks:
  - name: Show the normalized editor name
    log:
      message: "editor is ${{ editor | trim | lower }}"   # editor is nvim
```

## Where filters can be used

Filters are part of ironstate's expression language, so they work anywhere an expression is evaluated:

| Location | Example |
| --- | --- |
| `${{ }}` template expressions in any field | `package: "${{ jdk \| default('Microsoft.OpenJDK.25') }}"` |
| `when` conditions (bare, no `${{ }}`) | `when: editor \| default('vim') == 'nvim'` |
| `failed_when` conditions | `failed_when: stdout \| contains('ERROR')` |
| `vars` values that contain `${{ }}` | `editorconfig: "${{ vars.defaults.ec \| merge(ec) }}"` |

> [!NOTE]
> Inside a `template` module's **Jinja2** template body, the template engine's own filters apply. Gonja's `map`, for example, keeps its standard Jinja meaning. ironstate's filters apply to `${{ }}`, `when`, and `failed_when`.

## Syntax

```text
value | filter
value | filter(arg1, arg2, ...)
value | filter1 | filter2(arg) | filter3
filter(arg1, arg2, ...)              # bare call: the piped value is null
```

- **Arguments** are full expressions: literals (`'text'`, `42`, `true`, `null`), lists (`['a', 'b']`), maps (`{key: 'value'}`), variable paths (`vars.editor`, `item.name`), or other filter pipelines in parentheses.
- **Chaining** runs left to right: each filter receives the previous filter's result.
- **Precedence**: filters bind tighter than comparisons, `in`, and `is`, so `version | default('25') == '25'` runs the filter first, then compares.
- **Bare calls**: filters that don't use their input are usually called directly. `lookup('file', '~/.ssh/id_ed25519.pub')` is the same as `null | lookup(...)`.

## Null in, null out

Unless a filter's reference says otherwise, piping `null` into a filter returns `null` instead of an error. A missing variable can then flow through a chain safely and be caught at the end:

```yaml
message: "${{ maybe_missing | trim | upper | default('UNSET') }}"
```

Exceptions include `default`, `ternary`, `length`, `exists`, `enabled`, `map`, `merge`, `toggle`, `env`, and `contains`. Each one's behavior with `null` is described in [Built-in filters](./builtin).

## Common recipes

### Defaults

```yaml
# Use the user's value, or a fallback when it's missing or null
shell: "${{ vars.shell | default('bash') }}"

# Treat booleans as "use the package default", strings as an explicit override
package: "${{ languages.java.jdk | toggle('Microsoft.OpenJDK.25') }}"
```

### Merging defaults with user overrides

```yaml
vars:
  defaults:
    editorconfig: { indent_style: space, indent_size: 2 }
  settings: "${{ vars.defaults.editorconfig | merge(editorconfig) }}"
```

### Feature toggles

```yaml
# 'productivity' may be true, false, or a nested map of toggles
when: productivity | enabled('browsers', 'chrome')
```

### Paths

```yaml
dest: "${{ '~/.config' | join('nvim', 'init.lua') | resolve }}"
when: "'~/.cargo/bin/cargo' | exists"
```

### Fetching content

```yaml
blockinfile:
  path: ~/.ssh/authorized_keys
  block: "${{ lookup('url', 'https://github.com/', item.github_user, '.keys') }}"
  marker_name: "${{ item.github_user | sha1 }}"
```

### JSON

```yaml
when: "lookup('file', '~/.config/app.json') | from_json | json_query('.enabled')"
```

## Listing available filters

```shell
ironstate filters list --dir path/to/playbook/filters
```

This prints every built-in and discovered script filter. See [`ironstate filters`](../cli/commands/filters).

## Next

- [Built-in filters](./builtin): a reference for every filter that ships with ironstate.
- [Custom filters](./custom): write your own filters in PowerShell, bash, zsh, fish, or nushell.
