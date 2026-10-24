package model

import (
	"strings"
	"testing"
)

func TestNormalizeEnvironmentAliases(t *testing.T) {
	doc, err := Unmarshal([]byte(`
environment:
  A: one
envs:
  B: two
tasks:
  - environment:
      A: local
    actions:
      - envs:
          B: child
        cron:
          environment:
            JOB_VAR: job
`))
	if err != nil {
		t.Fatal(err)
	}
	if err := NormalizeDocumentEnvironment(doc); err != nil {
		t.Fatal(err)
	}
	root := AsMap(doc)
	if Envs(root)["A"] != "one" || Envs(root)["B"] != "two" {
		t.Fatalf("envs = %#v", Envs(root))
	}
	if _, exists := root["environment"]; exists {
		t.Fatal("alias was not canonicalized")
	}
	group := AsMap(AsList(root["tasks"])[0])
	if Envs(group)["A"] != "local" {
		t.Fatalf("group = %#v", group)
	}
	child := AsMap(AsList(group["actions"])[0])
	if AsMap(AsMap(child["cron"])["environment"])["JOB_VAR"] != "job" {
		t.Fatal("cron module environment was modified")
	}
}

func TestNormalizeEnvironmentRejectsConflictsAndInvalidMappings(t *testing.T) {
	for _, input := range []string{
		"environment: {A: one}\nenvs: {A: two}",
		"envs: value",
		"tasks:\n  - environment: []",
	} {
		t.Run(input, func(t *testing.T) {
			doc, err := Unmarshal([]byte(input))
			if err != nil {
				t.Fatal(err)
			}
			if err := NormalizeDocumentEnvironment(doc); err == nil {
				t.Fatal("invalid declaration accepted")
			}
		})
	}
	doc, _ := Unmarshal([]byte("environment: {A: one}\nenvs: {A: one}"))
	if err := NormalizeDocumentEnvironment(doc); err != nil {
		t.Fatalf("identical duplicate should be accepted: %v", err)
	}
	conflict, _ := Unmarshal([]byte("environment: {A: one}\nenvs: {A: two}"))
	if err := NormalizeDocumentEnvironment(conflict); err == nil || !strings.Contains(err.Error(), `"A"`) {
		t.Fatalf("conflict error = %v", err)
	}
}
