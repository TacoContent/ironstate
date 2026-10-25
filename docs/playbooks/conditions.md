---
render_with_liquid: false
layout: default
title: 🔀 Conditions and Expressions
nav_order: 7
parent: 📖 Playbooks
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 🔀 Conditions and Expressions
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
ironstate has one expression language, used in two places:

- **`when` / `failed_when`**: bare expressions, with no wrapper.
- **`${{ … }}`**: expressions embedded in any string field.

```yaml
- name: Install Steam
  when: facts.computer_name == 'krayt' and gaming | default(false)
  winget:
    package: "${{ steam_package | default('Valve.Steam') }}"
```

## `when`

`when` takes one expression, or a list of expressions that must **all** be true:

```yaml
when: facts.platform == 'linux'

when:
  - facts.platform == 'linux'
  - facts.os_family == 'archlinux'
  - not (skip_desktop | default(false))
```

A task's `when` combines with every enclosing group's `when` using AND. A false `when` skips the task, and in verbose mode (`-v`) it's reported as skipped.

## Operators

| Operator | Meaning |
| --- | --- |
| `==`, `!=`, `<`, `<=`, `>`, `>=` | Comparison. String comparison is **case-sensitive**. |
| `and`, `or`, `not` | Boolean logic. |
| `in`, `not in` | Membership in a list, or substring of a string. |
| `is`, `is not` | Type test (see below). |
| `\|` | Pipe into a [filter](../filters/). |
| `( … )` | Grouping. |

Precedence, from loosest to tightest: `or`, `and`, `not`, comparisons, `in` / `is`, `|`, then values.

### Values

| Kind | Examples |
| --- | --- |
| Strings | `'text'`, `"text"` |
| Numbers | `42`, `3.14` |
| Booleans and null | `true`, `false`, `null` |
| Lists | `['a', 'b', item.name]` |
| Maps | `{name: 'x', 'some-key': true, nested: {a: 1}}`. Keys are literal; values are expressions. |
| Paths | `facts.platform`, `languages.java.jdk`, `my_task.results[0].rc` |

A bare path with no operator is checked for truthiness: `when: enable_extras`.

### Type tests

| Test | True when the value is… |
| --- | --- |
| `defined` | set (not missing) |
| `none` / `null` | `null` or missing |
| `string` | a string |
| `number` | a number |
| `boolean` / `bool` | `true` or `false` |
| `mapping` / `map` | a map |
| `list` | a list |

Use type tests when a variable can hold a boolean **or** a map. A non-empty map is also "truthy", so `x == true` can't tell the two apart:

```yaml
when: (languages is mapping and languages.rust) or (languages is boolean and languages)
# or, more simply:
when: languages | enabled('rust')
```

## `${{ }}` templates

`${{ expression }}` can appear in any string field: module settings, `name`, `tags`, `items`, `vars`, and so on.

```yaml
- name: Link ${{ item.name }}
  symlinks:
    src: "${{ facts.local_bin_path | join(item.name) }}"
    dest: "~/bin/${{ item.name }}"
```

### Whole-field vs. embedded

- **Whole field** (`state: ${{ package.state }}`): the field takes the expression's native type, so lists stay lists and booleans stay booleans. If the expression **can't be resolved**, the field is **left out** with a warning, so the module's default applies.
- **Embedded in text** (`dest: "~/bin/${{ item.name }}"`): the result is inserted as text. An unresolved expression becomes an empty string.

> [!TIP]
> Quote values that **start** with `${{`, as in `"${{ … }}"`. Otherwise YAML may treat the `{` as the start of a map.

### What you can reference

| Expression | Value |
| --- | --- |
| `facts.<name>` | A gathered or custom fact. |
| `vars.<path>` / `<path>` | A variable. |
| `envs.<NAME>` | An environment variable. See [Environment variables](./environment). |
| `item`, `parent.item` | The current loop item, and the outer loop's item. See [Loops](./loops). |
| `<id>`, `<id>.<field>` | An earlier task's result. See [Results](./results). |
| `package.name`, `package.state`, `package.tags` | Inside an included package. See [Includes](./includes). |
| `inputs.<key>` | Inside an included package: the `include`'s `with` values. |

Use `[N]` to index into lists: `${{ example.results[0].stdout }}`.

### When expressions are resolved

Each leaf's expressions are resolved **just before that leaf runs**. A task can therefore use the result of any task above it. A task that hasn't run yet, or was skipped, resolves to `null`, so guard against it:

```yaml
when: check is defined and check.rc != 0
```
