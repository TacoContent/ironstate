
## [v0.3.0](https://github.com/tacocontent/ironstate/releases/tag/v0.3.0) - 2026-09-13

### 🚀 FEATURES

#### _GENERAL_

- Added async/wait_for handlers -[@camalot](https://github.com/camalot)

- Git handler added -[@camalot](https://github.com/camalot)

- Firewall / iptables / ufw / advfirewall handler support. -[@camalot](https://github.com/camalot)

- Support for user/group and handlers that manage files also support user/group. -[@camalot](https://github.com/camalot)

- Added init --scan that will scaffold a playbook based on the system. -[@camalot](https://github.com/camalot)

- Added mount facts -[@camalot](https://github.com/camalot)

- Apt support -[@camalot](https://github.com/camalot)

- Go 1.27 -[@camalot](https://github.com/camalot)

- Added other package managers: apk/pacman/snap/flatpak/scoop/mackports -[@camalot](https://github.com/camalot)

- Enable ssh enforcement for git -[@camalot](https://github.com/camalot)

- Added support for xget -[@camalot](https://github.com/camalot)

- feat: added support for xget -[@camalot](https://github.com/camalot)

- Plugin handler support implemented through phase 5 -[@camalot](https://github.com/camalot)

- Example plugin, documentation -[@camalot](https://github.com/camalot)

- Windows binary icon -[@camalot](https://github.com/camalot)

- Added manpage -[@camalot](https://github.com/camalot)

- feat: windows binary icon -[@camalot](https://github.com/camalot)

- feat: example plugin, documentation -[@camalot](https://github.com/camalot)

- feat: plugin handler support implemented through phase 5 -[@camalot](https://github.com/camalot)


### 🐛 BUG FIXES

#### _GENERAL_

- Remove test scan generated playbook that was at project root -[@camalot](https://github.com/camalot)

- Resolve the log output task to use the correct task -[@camalot](https://github.com/camalot)

- Status progress when applying / dry-run -[@camalot](https://github.com/camalot)

- Handlers now implement their own scanner for the scanning process of init -[@camalot](https://github.com/camalot)

- Update docs and everything to reference main.yml as the default for new playbook. site.yml still supported -[@camalot](https://github.com/camalot)

- Resolve order of expression resolution versus item parsing -[@camalot](https://github.com/camalot)

- Enable symlinks on gitconfig -[@camalot](https://github.com/camalot)

- fix: enable symlinks on gitconfig -[@camalot](https://github.com/camalot)

- Use xget for playbook -[@camalot](https://github.com/camalot)

- Goreleaser config -[@camalot](https://github.com/camalot)

- Build fixes -[@camalot](https://github.com/camalot)

- Govulcheck fixes -[@camalot](https://github.com/camalot)

- Windows test not skipped on linux -[@camalot](https://github.com/camalot)

- Windows test not skipped on linux -[@camalot](https://github.com/camalot)

- Generate sha256 for install scripts -[@camalot](https://github.com/camalot)

- fix: windows test not skipped on linux -[@camalot](https://github.com/camalot)

- fix: windows test not skipped on linux -[@camalot](https://github.com/camalot)

- fix: govulcheck fixes -[@camalot](https://github.com/camalot)

- fix: build fixes -[@camalot](https://github.com/camalot)

- Move manpage generation -[@camalot](https://github.com/camalot)


### 💼 OTHER

#### _GENERAL_

- Merge branch 'develop' of github.com:TacoContent/ironstate into develop -[@camalot](https://github.com/camalot)

- Develop' of github.com:TacoContent/ironstate: -[@camalot](https://github.com/camalot)

- Merge branch 'develop' of github.com:TacoContent/ironstate into develop -[@camalot](https://github.com/camalot)

- Develop' of github.com:TacoContent/ironstate: -[@camalot](https://github.com/camalot)

- Merge pull request  from TacoContent/plugins-core[#8](https://github.com/tacocontent/ironstate/issues/8)  [#8](https://github.com/tacocontent/ironstate/pull/8) -[@camalot](https://github.com/camalot)

- Plugins core [#8](https://github.com/tacocontent/ironstate/pull/8) -[@camalot](https://github.com/camalot)

- Version updates -[@camalot](https://github.com/camalot)

- Merge pull request  from TacoContent/plugins-core[#11](https://github.com/tacocontent/ironstate/issues/11)  [#11](https://github.com/tacocontent/ironstate/pull/11) -[@camalot](https://github.com/camalot)

- Version updates [#11](https://github.com/tacocontent/ironstate/pull/11) -[@camalot](https://github.com/camalot)

- Merge branch 'develop' of github.com:TacoContent/ironstate into develop -[@camalot](https://github.com/camalot)

- Develop' of github.com:TacoContent/ironstate: -[@camalot](https://github.com/camalot)

- deps: version updates -[@camalot](https://github.com/camalot)

- Merge branch 'develop' of github.com:TacoContent/ironstate into develop -[@camalot](https://github.com/camalot)

- Develop' of github.com:TacoContent/ironstate: -[@camalot](https://github.com/camalot)


### 📚 DOCUMENTATION

#### _GENERAL_

- Plugin implementation plan -[@camalot](https://github.com/camalot)

- Update changelog for v0.3.0 -[@github-actions[bot]](https://github.com/github-actions[bot])

- docs: update changelog for v0.3.0 -[@camalot](https://github.com/camalot)

- Update man page for v0.3.0

### ⚙️ MISCELLANEOUS TASKS

#### _GENERAL_

- Remove test playbook -[@camalot](https://github.com/camalot)

- Playbook changes -[@camalot](https://github.com/camalot)

- Add workflow to release existing tag -[@camalot](https://github.com/camalot)

- Revamp package / release process -[@camalot](https://github.com/camalot)

- Some tweaks to ci flow -[@camalot](https://github.com/camalot)

- chore: some tweaks to ci flow -[@camalot](https://github.com/camalot)

- Added empty changelog file -[@camalot](https://github.com/camalot)

- Some code cleanup -[@camalot](https://github.com/camalot)

- Task clean up and improve -[@camalot](https://github.com/camalot)


### ◀️ REVERT

#### _GENERAL_

- Revert "docs: update changelog for v0.3.0 -[@github-actions[bot]](https://github.com/github-actions[bot])

- This reverts commit 05cfaa771337337c40dc7f116e0d92cc0796e13d. -[@github-actions[bot]](https://github.com/github-actions[bot])

- Revert "docs: update changelog for v0.3.0 -[@camalot](https://github.com/camalot)


## GitHub

### ❤️ New Contributors

- [@github-actions[bot]](https://github.com/github-actions[bot])
### 💛 Contributors


- [@camalot](https://github.com/camalot)
## 📈 Commit Statistics


- `69` commits contributed to the release.
- `18` days have passed between the first and last commit.
- `43` commits parsed as conventional.
- `2` linked issues detected in commits.
  - [#11](https://github.com/tacocontent/ironstate/issues/11) (referenced 1 time)
  - [#8](https://github.com/tacocontent/ironstate/issues/8) (referenced 1 time)
- `18` days  have passed between releases.


![Statistics](https://quickchart.io/chart?c={type:'bar',data:{labels:['Commits','Contributors','Days%20Between%20Commits','Conventional%20Commits','Referenced%20Links','Days%20Since%20Last%20Release'],datasets:[{label:'Release',data:[69,2,18,43,2,18]}]}})



---


**Full Changelog**: https://github.com/tacocontent/ironstate/compare/v0.2.0...v0.3.0
