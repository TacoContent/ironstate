# advfirewall handler

The `advfirewall` handler manages Windows Firewall rules using `netsh advfirewall`.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `name` | yes | - | Rule name |
| `rule_name` | no | unset | Alias for `name` |
| `direction` | no | `in` | Rule direction (`in`/`out`) |
| `action` | no | `allow` | `allow` or block-like values (`block`, `deny`, `reject`, `drop`) |
| `protocol` | no | `ANY` | Protocol value (`ANY`, `TCP`, `UDP`, etc.) |
| `local_port` | no | unset | Local port/range |
| `remote_port` | no | unset | Remote port/range |
| `port` | no | unset | Alias for `local_port` |
| `source` | no | unset | Remote IP selector |
| `destination` | no | unset | Local IP selector |
| `program` | no | unset | Program path restriction |
| `profile` | no | unset | Profile (`domain`, `private`, `public`, `any`) |
| `enable` | no | `yes` | Netsh enable flag |
| `state` | no | `present` | `present` / `absent` / `latest` |

## Behavior

- `present` / `latest`: adds the named rule.
- `absent`: deletes rule(s) matching `name`.
- This handler is Windows-only.

## Example

```yaml
tasks:
  - name: allow winrm inbound
    advfirewall:
      name: "Allow WinRM 5985"
      direction: in
      action: allow
      protocol: TCP
      local_port: "5985"
      state: present
```
