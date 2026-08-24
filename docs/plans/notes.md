# Notes

- allow loading of a variables file from flag `--vars-file`
- allow automatic loading of a variables file based on hostname, architecture, os family, platform.
- allow combo of both `--vars-file` and automatic loading based on system characteristics
- allow "chained" name like: `hostname.architecture.os_family.platform.yml` or `hostname.architecture.yml` or `hostname.yml` or any combination of these based on available system characteristics.
  - hosts/ path should also be able to automatically load tasks file based on system characteristics.
  - other paths should support automatic loading of tasks files based on system characteristics as well. if the task file is "loaded" by using an `include` for example. 

  ``` yaml
    - include:
      name: hosts/camalot
  ```

  this should load `main.yml`, and then more specific files based on system characteristics if they exist.

- allow "default" variables file like `main.yml` to be loaded if no specific file is found based on system characteristics.
- allow overriding of specific variables through command-line flags. `--var key=value`


---

- change `--file` to be `--playbook`. it should not require the path to the file specifically.
  - `--playbook=playbooks/camalot` for example should be supported as well as `--playbook=playbooks/camalot/main.yml`
  - it should support looking for `site.{yml, yaml}` or `main.{yml, yaml}` in the no file is specified.
  - if a specific playbook is provided, it should use that instead of the default files.
  - if no playbook is found, it should raise an error indicating that no playbook could be located.
