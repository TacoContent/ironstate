# ufw handler

The `ufw` handler manages firewall rules using the `ufw` CLI.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `rule` | no | `allow` | UFW rule operation: `allow`, `deny`, `reject`, `limit` |
| `action` | no | unset | Alias for `rule` intent |
| `direction` | no | unset | `in` or `out` |
| `interface` | no | unset | Interface constraint (`on <iface>`) |
| `source` | no | `any` | `from` selector |
| `destination` | no | `any` | `to` selector |
| `port` | no | unset | Port/range |
| `protocol` | no | unset | Protocol (`tcp`, `udp`, etc.) |
| `comment` | no | unset | Rule comment (when supported by local ufw) |
| `state` | no | `present` | `present` / `absent` / `latest` |

## Behavior

- `present` / `latest`: applies the rule (`ufw --force ...`).
- `absent`: deletes the rule (`ufw --force delete ...`).
- Deleting a non-existent rule is normalized as success.

## Example

```yaml
tasks:
  - name: allow https inbound
    ufw:
      rule: allow
      direction: in
      port: "443"
      protocol: tcp
      state: present
```
