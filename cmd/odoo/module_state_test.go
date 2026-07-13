package odoo

import "testing"

func TestParseModuleStateOutputWithLogPrefix(t *testing.T) {
	states, err := parseModuleStateOutput("2026-01-01 INFO startup\n[{\"name\":\"sale\",\"state\":\"installed\"}]")
	if err != nil {
		t.Fatalf("parseModuleStateOutput() error = %v", err)
	}
	if len(states) != 1 || states[0].Name != "sale" || states[0].State != "installed" {
		t.Fatalf("unexpected states: %#v", states)
	}
}
