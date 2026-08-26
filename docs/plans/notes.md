# Notes

## Possible Handlers

- [cron](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/cron_module.html#ansible-collections-ansible-builtin-cron-module): manage cron jobs (Linux/macOS) or alias for scheduled tasks (Windows)
- [find](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/find_module.html#ansible-collections-ansible-builtin-find-module): find files on the target system
- [get_url](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/get_url_module.html#ansible-collections-ansible-builtin-get-url-module): download a file from a HTTP/HTTPS/FTP
- [group](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/group_module.html#ansible-collections-ansible-builtin-group-module): manage groups on the target system
- [user](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/user_module.html#ansible-collections-ansible-builtin-user-module): manage users on the target system
- [hostname](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/hostname_module.html#ansible-collections-ansible-builtin-hostname-module): manage the hostname of the target system
- firewall: manage firewall rules (Linux/macOS) or alias for Windows firewall management
  This handler is a wrapper around the `iptables` and `ufw` handlers on Linux/macOS, and the `netsh advfirewall` handler on Windows. It automatically detects the platform and uses the appropriate underlying handler.
  - [iptables](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/iptables_module.html#ansible-collections-ansible-builtin-iptables-module): manage iptables rules (Linux)
  - ufw: manage ufw rules (Linux)
  - netsh advfirewall: manage Windows firewall rules
<!-- - [known_hosts](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/known_hosts_module.html#ansible-collections-ansible-builtin-known-hosts-module): manage SSH known hosts -->
- [mount_facts](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/mount_facts_module.html#ansible-collections-ansible-builtin-mount-facts-module): gather facts about mounted filesystems
- [package](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/package_module.html#ansible-collections-ansible-builtin-package-module): manage packages (Linux/macOS) or alias for Windows package management
- choice: prompt the user to choose from a list of options (Linux/macOS) or alias for Windows pause
- service: manage services (Linux/macOS) or alias for Windows service management
- service_facts: gather facts about services (Linux/macOS) or alias for Windows service management
- tempfile: create a temporary file or directory (Linux/macOS) or alias for Windows tempfile

- unarchive: extract an archive (Linux/macOS) or alias for Windows unarchive
- async: run a task or group of tasks asynchronously, allowing the playbook to continue without waiting for the task to finish. Can be used with `wait_for` to check for completion.
- wait_for: wait for a fact or condition to be true or async task(s) to complete, then continue. Timeout can be specified, and the task will fail if the condition is not met in time.

## GitHub

- create issue templates
