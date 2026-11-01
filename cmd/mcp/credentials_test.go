package mcp

import (
	"os"
	"strings"
	"testing"
)

func TestBrowserCredentials(t *testing.T) {
	for _, tc := range []struct {
		name, login, password, wantLogin, wantPassword string
		authenticate, unset                            bool
	}{
		{name: "unset defaults", authenticate: true, unset: true, wantLogin: "admin", wantPassword: "admin"},
		{name: "empty defaults", authenticate: true, wantLogin: "admin", wantPassword: "admin"},
		{name: "login override", authenticate: true, login: "developer", wantLogin: "developer", wantPassword: "admin"},
		{name: "password override", authenticate: true, password: "fixture-password", wantLogin: "admin", wantPassword: "fixture-password"},
		{name: "both overrides", authenticate: true, login: "developer", password: "fixture-password", wantLogin: "developer", wantPassword: "fixture-password"},
		{name: "preserve nonempty values", authenticate: true, login: " developer ", password: " fixture-password ", wantLogin: " developer ", wantPassword: " fixture-password "},
		{name: "public ignores defaults"},
		{name: "public ignores overrides", login: "developer", password: "fixture-password"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ODOOCTL_MCP_BROWSER_LOGIN", tc.login)
			t.Setenv("ODOOCTL_MCP_BROWSER_PASSWORD", tc.password)
			if tc.unset {
				for _, key := range []string{"ODOOCTL_MCP_BROWSER_LOGIN", "ODOOCTL_MCP_BROWSER_PASSWORD"} {
					if err := os.Unsetenv(key); err != nil {
						t.Fatal(err)
					}
				}
			}
			login, password := browserCredentials(tc.authenticate)
			if login != tc.wantLogin || password != tc.wantPassword {
				t.Fatal("credential resolution did not match defaults, overrides or authentication mode")
			}
			if os.Getenv("ODOOCTL_MCP_BROWSER_LOGIN") != tc.login || os.Getenv("ODOOCTL_MCP_BROWSER_PASSWORD") != tc.password {
				t.Fatal("credential resolution modified the environment")
			}
		})
	}
}

func TestBrowserCommandsResolveDefaultsBeforeRuntimeLookup(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ODOOCTL_MCP_BROWSER_LOGIN", "")
	t.Setenv("ODOOCTL_MCP_BROWSER_PASSWORD", "")
	project := t.TempDir()
	for _, args := range [][]string{
		{"serve", "--browser", "--project", project},
		{"browser", "--project", project},
	} {
		cmd := NewCommand("test")
		cmd.SetArgs(args)
		err := cmd.Execute()
		if err == nil {
			t.Fatal("command unexpectedly accepted a project without a runtime")
		}
		if strings.Contains(err.Error(), "requires both") || strings.Contains(err.Error(), "requires ODOOCTL_MCP_BROWSER") {
			t.Fatal("CLI did not provide its development defaults")
		}
	}
}
