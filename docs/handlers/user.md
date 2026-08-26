# user

The `user` handler manages local users on Windows, Linux, and macOS.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `name` | yes | unset | User name |
| `password` | no | unset | Password value (platform behavior varies) |
| `password_env` | no | unset | Environment variable name containing password |
| `uid` | no | unset | User id on Unix/macOS |
| `group` / `gid` | no | unset | Primary group (name or gid) |
| `groups` | no | unset | Supplementary groups |
| `shell` | no | unset | Login shell |
| `home` | no | unset | Home directory |
| `comment` | no | unset | Display/full name/comment |
| `system` | no | `false` | Create as a system account where supported |
| `create_home` | no | `true` | Linux: create home directory |
| `remove_home` | no | `false` | Linux: remove home directory on delete |
| `state` | no | `present` | `present` / `absent` / `latest` |

## Platform behavior

- Windows: uses `net user` and `net localgroup`.
- Linux: uses `id`, `useradd`, and `userdel`.
- macOS: uses `dscl`, `sysadminctl`, and `dseditgroup`.
