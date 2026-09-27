package docker

import "testing"

func TestCreateDoesNotAutoDiscoverDepsByDefault(t *testing.T) {
	flag := createCmd.Flags().Lookup("auto-discover-deps")
	if flag == nil {
		t.Fatal("auto-discover-deps flag missing")
	}
	if flag.DefValue != "false" {
		t.Fatalf("auto-discover-deps default = %q, want false", flag.DefValue)
	}
}

func TestCreateHasPresetFlag(t *testing.T) {
	flag := createCmd.Flags().Lookup("preset")
	if flag == nil {
		t.Fatal("preset flag missing")
	}
}
