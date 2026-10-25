---
render_with_liquid: false
layout: default
title: 🧱 Built-in
nav_order: 1
parent: 🧹 Filters
---
<!-- markdownlint-disable-next-line MD025 MD022 -->
# 🧱 Built-in Filters
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
Every filter below ships with ironstate and is always available. Built-ins take priority over [custom script filters](./custom) with the same name.

Unless noted, a filter is **null-in/null-out**: piping `null` returns `null`.

## Quick reference

| Filter | Arguments | Summary |
| --- | --- | --- |
| [`default`](#default) | `fallback` | Value, or `fallback` when null. |
| [`toggle`](#toggle) | `fallback` | Value if it's a string, else `fallback`. |
| [`ternary`](#ternary) | `whenTrue, whenFalse` | Pick one of two values by truthiness. |
| [`enabled`](#enabled) | `...keys` | Walk an on/off toggle tree. |
| [`map`](#map) | none | Build or copy a map. |
| [`merge`](#merge) | `...maps` or `...lists` | Deep-merge maps or concatenate lists. |
| [`upper`](#upper--lower) / [`lower`](#upper--lower) | none | Change case. |
| [`trim`](#trim) | none | Strip leading and trailing whitespace. |
| [`quote`](#quote) | `quoteChar?` | Wrap in quotes. |
| [`length`](#length) | none | String or list length. |
| [`contains`](#contains) | `substring` | Substring test. |
| [`concat`](#concat) | `delimiter, ...extra` | Join items into a string. |
| [`split`](#split) | `delimiter` | Split a string into a list. |
| [`prefix`](#prefix) | `text` | Prepend `text ` to a value or to each item. |
| [`join`](#join) | `...segments` | Combine filesystem path segments. |
| [`dirname`](#dirname--basename) / [`basename`](#dirname--basename) | none | Path parent or file name. |
| [`resolve`](#resolve) | none | Expand a leading `~`. |
| [`exists`](#exists) | `expected?` | Test whether a path exists. |
| [`extension`](#extension) | `extensions, add?` | Ensure a file extension (Windows only). |
| [`sha1`](#sha1) | none | SHA-1 hex digest. |
| [`env`](#env) | `key, default?` | Read an environment variable. |
| [`lookup`](#lookup) | `action, ...pieces` | Read an env var, file, or URL. |
| [`from_json`](#from_json) | none | Parse a JSON string. |
| [`json_query`](#json_query) | `query` | Query parsed JSON. |

## Values and defaults

### `default`

`value | default(fallback)`. Returns the value if it isn't `null`; otherwise returns `fallback`. Empty strings, `false`, and `0` are **not** replaced.

```yaml
shell: "${{ vars.shell | default('bash') }}"
```

### `toggle`

`value | toggle(fallback)`. Returns the value if it's a **string**; otherwise returns `fallback`. This handles variables that can be either an on/off boolean or a string naming a specific choice:

```yaml
# languages.java.jdk: true                   -> Microsoft.OpenJDK.25
# languages.java.jdk: 'Eclipse.Temurin.21'   -> Eclipse.Temurin.21
- name: Install Java
  winget:
    package: "${{ languages.java.jdk | toggle('Microsoft.OpenJDK.25') }}"
  when: languages.java.jdk != false
```

`toggle` only picks the value. Use a `when` to skip the task when the toggle is `false`.

### `ternary`

`value | ternary(whenTrue, whenFalse)`. Returns `whenTrue` if the value is truthy, otherwise `whenFalse`. `null` is falsy, so this filter always returns one of its arguments.

```yaml
state: "${{ install_docker | ternary('present', 'absent') }}"
```

### `enabled`

`value | enabled('key1', 'key2', ...)`. Walks a toggle tree key by key, where each level can be a boolean or a map:

- A **boolean** at any level ends the walk and is the answer. `true` enables everything below it, and `false` disables everything below it.
- A **map** keeps descending.
- When the keys run out, a `true` value **or a map** counts as enabled.
- Anything else (a missing key, a string, a number, or `null`) is `false`.

```yaml
vars:
  productivity:
    browsers:
      chrome: true
      firefox: false
    office: false

# productivity | enabled('browsers', 'chrome')    -> true
# productivity | enabled('browsers', 'firefox')   -> false
# productivity | enabled('browsers')              -> true  (a map is "configured")
# productivity | enabled('office', 'word')        -> false (office: false wins)
```

Pass the **root** variable and the keys as arguments. If you write `productivity.office.word | enabled`, the variable path resolves before the filter runs, so the filter can't see that a `false` ancestor disabled it.

### `map`

`value | map`, or `map()`. Converts to a map. Takes no arguments.

| Input | Result |
| --- | --- |
| `null` / `map()` | `{}` |
| a map | a deep copy |
| a JSON object string | the parsed map |
| a list of `[key, value]` pairs | a map (when a key repeats, the last pair wins) |

Any other input is an error.

```yaml
message: "${{ '{\"a\": 1}' | map | merge({b: 2}) }}"
```

### `merge`

`value | merge(other, ...)`. Combines maps or lists without changing the inputs.

- **Maps** merge recursively, from left to right. When keys conflict, later values win. Nested lists and scalars, including explicit `null`, **replace** the earlier value.
- **Lists** concatenate in order, keeping duplicates.
- `null` inputs or arguments count as empty. Mixing maps and lists is an error.

```yaml
vars:
  defaults:
    editorconfig: { indent_style: space, indent_size: 2, trim: true }
  # user value: { indent_size: 4 }
  editorconfig_final: "${{ vars.defaults.editorconfig | merge(editorconfig) }}"
  # -> { indent_style: space, indent_size: 4, trim: true }

  all_packages: "${{ base_packages | merge(extra_packages, ['jq']) }}"
```

To fall back to the defaults only when the user value is missing (all or nothing, with no partial merge), use `default` instead.

## Strings

### `upper` / `lower`

Uppercase or lowercase. Non-string values are converted to strings first.

### `trim`

Removes leading and trailing whitespace.

### `quote`

`value | quote` or `value | quote("'")`. Wraps the value in a quote character (`"` by default). A blank or whitespace-only value becomes `null` instead of `""`, so `quote | default(...)` still works.

```yaml
command: "code --install-extension ${{ item | quote }}"
```

### `length`

The character count of a string, or the item count of a list. Returns `0` for `null` or any other type.

```yaml
when: extensions | length > 0
```

### `contains`

`value | contains(substring)`. `true` if the value is a string containing `substring`. Any non-string value, including `null`, returns `false`.

```yaml
failed_when: stderr | contains('permission denied')
```

The `in` operator does the same check: `'denied' in stderr`.

### `concat`

`value | concat(delimiter, ...extra)`. Joins the value into one string. A list value has its items joined; a scalar counts as a single item. Any `extra` arguments are appended before joining.

```yaml
"hello" | concat(" ", "world")         # "hello world"
['a', 'b'] | concat(",")               # "a,b"
concat(envs.HOME, "/", "bin")          # bare call -> pieces after the delimiter
```

### `split`

`value | split(delimiter)`. Splits a string on a literal delimiter, giving a list. One trailing empty item is dropped, so `concat` and `split` round-trip.

```yaml
with: "${{ item.keys | split('\n') }}"
```

### `prefix`

`value | prefix(text)`. Prepends `text` followed by one space to a string, or to every item of a list.

```yaml
# "host1 host2 ssh-ed25519 AAAA..." for each key line
"${{ item.key | split('\n') | prefix(item.hostnames | concat(' ')) }}"
```

### `sha1`

Lowercase hex SHA-1 digest of a string. Use it to build stable, unique identifiers from content, such as a `blockinfile` `marker_name`.

```yaml
marker_name: "${{ item.public_key | sha1 }}"
```

## Paths

### `join`

`value | join(segment, ...)`. Combines the value and arguments as **filesystem path segments**, using the platform separator. To join a list into a delimited string, use [`concat`](#concat).

```yaml
dest: "${{ facts.home | join('.config', 'nvim', 'init.lua') }}"
```

### `dirname` / `basename`

The parent directory or the file name of a path string. Both `/` and `\` are separators. `dirname` returns `""` when there's no separator.

```yaml
"~/bin/tool.exe" | dirname     # "~/bin"
"~/bin/tool.exe" | basename    # "tool.exe"
```

### `resolve`

Expands a leading `~` (`~`, `~/...`, or `~\...`) to the current user's home directory. Other paths pass through unchanged. Takes no arguments.

```yaml
dest: "${{ '~/.local/bin' | resolve }}"
```

### `exists`

`value | exists` or `value | exists(false)`. Checks whether a path (with `~` expanded) exists on disk. Returns `expected` (default `true`) when it exists, and `not expected` when it doesn't. A `null`, blank, or non-string value counts as "doesn't exist".

```yaml
when: "'~/.oh-my-zsh' | exists(false)"   # only when it's missing
```

### `extension`

`value | extension(extensions, add?)`. **Windows only**; on other platforms the value passes through unchanged. If the path doesn't already end in one of `extensions` (a string or list, with or without the leading dot, case-insensitive), it appends `add`. If `add` is omitted, it appends the first entry of `extensions`.

```yaml
# "tool" -> "tool.exe" on Windows, "tool" elsewhere
exe: "${{ item.name | extension(['exe', 'cmd'], 'exe') }}"
```

## Environment and external content

### `env`

`env(key)` or `env(key, default)`. Returns the environment variable `key` for the ironstate process. That includes values loaded from `.env` and `.secrets`, and run-level `envs`. With a `default`, the default is returned when the variable is unset or empty. The piped value is ignored, so this is usually a bare call.

```yaml
message: "${{ env('EDITOR', 'vim') }}"
```

### `lookup`

`lookup(action, piece, ...)`. Reads external content. Every argument after `action` is **concatenated** to form the target. If **any** piece is `null` or empty, the whole lookup returns `null` instead of building a broken target, so a missing per-item value skips the lookup cleanly.

| Action | Result |
| --- | --- |
| `env` | The environment variable named by the joined pieces (empty when unset). |
| `file` | The contents of a local file (`~` is expanded). A missing file returns `null`. |
| `url` | The response body of an HTTP `GET`. |

```yaml
block: "${{ lookup('url', 'https://github.com/', item.github_user, '.keys') }}"
block: "${{ lookup('file', '~/.ssh/id_ed25519.pub') }}"
value: "${{ lookup('env', 'MY_TOKEN') }}"
```

**Headers for `url`**: when there are at least two pieces and the last one is a map or a list of maps, it's sent as request headers instead of being part of the URL. Headers with `null` or empty values are skipped. A `null` headers argument means "no headers", so an optional token doesn't block the request.

```yaml
vars:
  api_headers:
    Accept: application/vnd.github+json
    Authorization: "${{ lookup('env', 'GITHUB_TOKEN') | prefix('Bearer') }}"

tasks:
  - name: Fetch release metadata
    fact:
      name: latest_release
      value: "${{ lookup('url', 'https://api.github.com/repos/', repo, '/releases/latest', vars.api_headers) | from_json | json_query('.tag_name') }}"
```

> [!NOTE]
> During a [remote apply](../playbooks/inventory), `lookup` runs **on the target**, not the controller.

### `from_json`

Parses a JSON string into a value (map, list, string, number, boolean, or null). Parse errors include the start of the input to help with debugging.

### `json_query`

`value | json_query(query)`. Extracts data from a parsed JSON value, usually piped from `from_json`.

- If [`jq`](https://jqlang.github.io/jq/) is on `PATH`, `query` is a full `jq` program (for example, `.ssh_keys[0]` or `[.items[] | .name]`). Results that are valid JSON are returned as values; anything else is returned as raw text.
- Without `jq`, only a single bare key lookup on a map is supported (for example, `json_query('ssh_keys')`).

```yaml
keys: "${{ lookup('url', api_url) | from_json | json_query('.ssh_keys') }}"
```
