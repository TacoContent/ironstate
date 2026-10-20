package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestIncludedTemplateDefaults(t *testing.T) {
	for _, tc := range []struct {
		name, siteVars, expression, want string
	}{
		{"missing namespace", "{}", "", "[*]\nindent_size = 2\n"},
		{"missing option", "{development: {other: true}}", "", "[*]\nindent_size = 2\n"},
		{"explicit null", "{development: {editorconfig: null}}", "", "[*]\nindent_size = 2\n"},
		{"user override", "{development: {editorconfig: {rules: {'*': {indent_size: 4}}}}}", "", "[*]\nindent_size = 4\n"},
		{"site defaults override", "{defaults: {development: {editorconfig: {rules: {'*': {indent_size: 6}}}}}}", "", "[*]\nindent_size = 6\n"},
		{"merged defaults", "{}", "vars.defaults.development.editorconfig | merge(development.editorconfig)", "[*]\nindent_size = 2\n"},
		{"merged partial override", "{development: {editorconfig: {rules: {'*': {charset: utf-8}}}}}", "vars.defaults.development.editorconfig | merge(development.editorconfig)", "[*]\ncharset = utf-8\nindent_size = 2\n"},
		{"chained literal override", "{development: {editorconfig: {rules: {'*': {charset: utf-8}}}}}", "vars.defaults.development.editorconfig | merge(development.editorconfig) | merge({rules: {'*': {indent_style: 'space'}}})", "[*]\ncharset = utf-8\nindent_size = 2\nindent_style = space\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			pkgDir := filepath.Join(dir, "packages", "editorconfig")
			if err := os.MkdirAll(pkgDir, 0o700); err != nil {
				t.Fatal(err)
			}
			expression := tc.expression
			if expression == "" {
				expression = "development.editorconfig | default(vars.defaults.development.editorconfig)"
			}
			files := map[string]string{
				"main.yml": "vars: " + tc.siteVars + "\ntasks:\n  - include:\n      name: packages/editorconfig\n",
				filepath.Join("packages", "editorconfig", "main.yml"): `vars:
  defaults:
    development:
      editorconfig:
        rules:
          '*':
            indent_size: 2
tasks:
  - template:
      src: editorconfig.j2
      dest: rendered.editorconfig
      engine: jinja2
      vars:
        editorconfig: "${{ ` + expression + ` }}"
`,
				filepath.Join("packages", "editorconfig", "editorconfig.j2"): `{% for pattern, rules in editorconfig.rules.items() %}[{{ pattern }}]
{% for key, value in rules.items() %}{{ key }} = {{ value }}
{% endfor %}{% endfor %}`,
			}
			for name, content := range files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			cmd, err := newRootCommand()
			if err != nil {
				t.Fatal(err)
			}
			cmd.SetArgs([]string{"--playbook", filepath.Join(dir, "main.yml"), "--apply", "--output", "json"})
			cmd.SetOut(new(bytes.Buffer))
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(dir, "rendered.editorconfig"))
			if err != nil || string(data) != tc.want {
				t.Fatalf("rendered = %q, err = %v, want %q", data, err, tc.want)
			}
		})
	}
}
