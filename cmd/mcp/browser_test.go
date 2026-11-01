package mcp

import "testing"

func TestBrowserCommandsExposeExplicitSetupAndPublicMode(t *testing.T) {
	root := NewCommand("test")
	browser, _, err := root.Find([]string{"browser"})
	if err != nil || browser.Name() != "browser" || browser.RunE == nil {
		t.Fatal("missing browser stdio command")
	}
	if browser.PersistentFlags().Lookup("project") == nil || browser.Flags().Lookup("public") == nil {
		t.Fatal("missing project/public flags")
	}
	for _, name := range []string{"setup", "stop"} {
		cmd, _, err := root.Find([]string{"browser", name})
		if err != nil || cmd.Name() != name || cmd.RunE == nil {
			t.Fatalf("missing browser %s command", name)
		}
	}
	setup, _, _ := root.Find([]string{"browser", "setup"})
	if setup.Flags().Lookup("allow-origin") == nil || browser.Flags().Lookup("allow-origin") != nil {
		t.Fatal("origins must be configured during explicit setup, not by inspection clients")
	}
	for _, flag := range []string{"password", "login", "allow-interactions", "allow-code", "allow-unrestricted-file-access"} {
		if browser.Flags().Lookup(flag) != nil {
			t.Fatalf("unsafe or credential flag advertised: %s", flag)
		}
	}
}
