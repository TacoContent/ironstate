# iptables handler

The `iptables` handler manages firewall rules using `iptables` (or `ip6tables` when `ipv6: true`).

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `chain` | no | `INPUT` (or `OUTPUT` for `direction: out`) | Target chain |
| `table` | no | `filter` | iptables table |
| `jump` | no | derived from `action` | Jump target (e.g. `ACCEPT`, `DROP`, `REJECT`) |
| `action` | no | `allow` | Convenience intent: `allow`/`deny`/`reject` |
| `direction` | no | `in` | Hint used when `chain` is omitted |
| `protocol` | no | unset | Protocol (`tcp`, `udp`, etc.) |
| `source` | no | unset | Source CIDR/address |
| `destination` | no | unset | Destination CIDR/address |
| `source_port` | no | unset | Source port/range |
| `destination_port` | no | unset | Destination port/range |
| `port` | no | unset | Alias for `destination_port` |
| `in_interface` | no | unset | Input interface selector |
| `out_interface` | no | unset | Output interface selector |
| `rule_num` | no | unset | Insert index when adding (uses `-I`) |
| `matches` | no | unset | Extra `-m` match modules |
| `comment` | no | unset | Rule comment (`-m comment --comment`) |
| `ipv6` | no | `false` | Use `ip6tables` instead of `iptables` |
| `state` | no | `present` | `present` / `absent` / `latest` |

## Behavior

- `present` / `latest`: ensures the rule exists (`iptables -C` probe, then `-A` or `-I` if missing).
- `absent`: ensures the rule is removed (`iptables -D`).

## Example

```yaml
tasks:
  - name: allow ssh inbound
    iptables:
      chain: INPUT
      protocol: tcp
      port: "22"
      action: allow
      state: present
```
