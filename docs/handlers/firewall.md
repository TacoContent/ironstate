# firewall handler

The `firewall` handler is a cross-platform wrapper that translates one normalized rule shape into a platform-specific backend handler.

It does not call a firewall CLI directly. Instead it routes to one of these handlers:

- `advfirewall` on Windows
- `ufw` on Linux/macOS when `ufw` is available
- `iptables` on Linux/macOS as a fallback when `ufw` is not available

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `backend` | no | `auto` | Backend override: `auto`, `ufw`, `iptables`, `advfirewall` |
| `name` | no | unset | Rule name (required by `advfirewall`) |
| `rule` | no | `allow` | Rule action (`allow`, `deny`, `reject`, `limit`) |
| `action` | no | unset | Alias for `rule` |
| `direction` | no | unset | Direction (`in`, `out`) |
| `source` | no | `any` | Source selector |
| `destination` | no | `any` | Destination selector |
| `port` | no | unset | Port/range |
| `protocol` | no | unset | Protocol (`tcp`, `udp`, etc.) |
| `interface` | no | unset | Interface selector (backend support varies) |
| `state` | no | `present` | `present` / `absent` / `latest` |

## Backend selection

When `backend` is omitted (or set to `auto`), the wrapper chooses automatically:

- Windows: `advfirewall`
- Linux/macOS: `ufw` if found on `PATH`, otherwise `iptables`

If a specific backend is requested and is not valid for the current platform, the task fails with a clear error.

## Behavior

- `present` / `latest`: delegates to the selected backend's install path.
- `absent`: delegates to the selected backend's uninstall path.
- Field translation is backend-aware (for example, `rule`/`action` and direction/port/protocol mappings).

## Examples

```yaml
tasks:
  - name: allow ssh (auto backend)
    firewall:
      direction: in
      rule: allow
      port: "22"
      protocol: tcp
      state: present

  - name: deny outbound smtp with explicit backend
    firewall:
      backend: ufw
      direction: out
      rule: deny
      port: "25"
      protocol: tcp
      state: present

  - name: windows rule with explicit name
    firewall:
      backend: advfirewall
      name: Allow-HTTPS-Inbound
      direction: in
      rule: allow
      port: "443"
      protocol: tcp
      state: present
```