# group

The `group` handler manages local groups on Windows, Linux, and macOS.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `name` | yes | unset | Group name |
| `gid` | no | unset | Group id on Unix/macOS |
| `system` | no | `false` | Create as a system group where supported |
| `state` | no | `present` | `present` / `absent` / `latest` |

## Platform behavior

- Windows: uses `net localgroup`.
- Linux: uses `getent group`, `groupadd`, and `groupdel`.
- macOS: uses `dscl`/`dseditgroup`.
