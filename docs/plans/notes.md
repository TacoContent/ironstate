# Notes

## Possible Handlers

- cross-handler metadata abstraction needed for file/folder modules: shared `owner`/`group` (and `mode`) application helper so handlers do not duplicate ownership code

- [find](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/find_module.html#ansible-collections-ansible-builtin-find-module): find files on the target system
- [get_url](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/get_url_module.html#ansible-collections-ansible-builtin-get-url-module): download a file from a HTTP/HTTPS/FTP
- [hostname](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/hostname_module.html#ansible-collections-ansible-builtin-hostname-module): manage the hostname of the target system
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

## Generate

a command that scans the system and generates a playbook around the current state of the system, including installed packages, users, groups, services, and other relevant configuration. This can be useful for creating a baseline or for migrating to a new system.

should support scanning for all the supported handlers to generate a comprehensive playbook that reflects the current state of the system. The generated playbook can then be used to replicate the configuration on other systems or to maintain consistency across multiple environments.

create using the roles/tasks/packages/hosts/variables structure, with roles created for the different types of configuration (e.g., users, groups, services, packages). Each role can contain tasks that manage the specific configuration items, and variables can be used to parameterize the playbook for different environments. You can look at the playbooks/camalot as an example. it doesn't have to be as complex, but it should be structured in a way that makes it easy to understand and maintain.

for example, to find all the symlinks that exist on a system, you can use the `find` command with the `-type l` option to search for symbolic links. You can then use the output of this command to generate a playbook that manages these symlinks, including creating, updating, or removing them as needed. for windows, you can use: 

```powershell
Get-ChildItem -Path "C:\" -Recurse -Force -ErrorAction SilentlyContinue | 
Where-Object { $_.LinkType } | 
Select-Object Name, LinkType, Target, FullName
```

This should also loop over all disks though as well. there should be known ignored paths. like anything in c:\windows, c:\program files, c:\program files (x86), c:\programdata, etc. for linux/mac, it should ignore /proc, /sys, /dev, /run, /tmp, /var/tmp, /var/run, etc.

this full process will take some time to complete. it should ignore any errors (like permission denied) and continue scanning the rest of the system. it should also be able to handle large numbers of files and directories without running out of memory or crashing.

there should be a "spinner" showing progress and what is currently being scanned. it should be a nice user experience, and it should be clear that the process is still running and not stuck. it should also show an estimated time remaining based on the number of files scanned and the total number of files to be scanned.

what is scanned, and how it is scanned should be extensible and configurable so that new handlers can be added in the future without having to modify the core scanning logic. this can be done by defining a set of interfaces or abstract classes that each handler must implement, and then using reflection or a plugin system to discover and load the handlers at runtime.

We can start with a simple implementation that scans for users, groups, services and installed packages from something like winget/chocolatey (on windows), npm, but we dont yet support any "system" package managers on linux/macos, and then gradually add support for other handlers as needed.

The output should be a directory structure that reflects the roles/tasks/packages/hosts/variables structure, with each role containing tasks that manage the specific configuration items. The generated playbook should be easy to read and understand, and it should be clear what each task does and how it relates to the overall configuration of the system.
