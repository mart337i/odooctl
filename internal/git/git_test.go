package git

import "testing"

func TestVersionFromBranch(t *testing.T) {
	for _, test := range []struct {
		branch string
		want   string
	}{
		{branch: "20.0", want: "20.0"},
		{branch: "20.0-feature", want: "20.0"},
		{branch: "19.0", want: "19.0"},
		{branch: "12.0-feature", want: "12.0"},
		{branch: "21.0"},
		{branch: "main"},
		{branch: ""},
	} {
		t.Run(test.branch, func(t *testing.T) {
			if got := VersionFromBranch(test.branch); got != test.want {
				t.Fatalf("VersionFromBranch(%q) = %q, want %q", test.branch, got, test.want)
			}
		})
	}
}
