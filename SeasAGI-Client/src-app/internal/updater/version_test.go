package updater

import "testing"

func TestParseVersion(t *testing.T) {
	cases := []struct {
		in   string
		want [3]int
	}{
		{"1.2.3", [3]int{1, 2, 3}},
		{"v1.2.3", [3]int{1, 2, 3}},
		{"v0.2", [3]int{0, 2, 0}},
		{"2", [3]int{2, 0, 0}},
		{"garbage", [3]int{0, 0, 0}},
		{"1.2.3.4", [3]int{1, 2, 3}},
	}
	for _, c := range cases {
		if got := parseVersion(c.in); got != c.want {
			t.Errorf("parseVersion(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestVersionGreaterThan(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1.0.0", "0.1.5", true},
		{"v0.2.0", "0.1.5", true},
		{"0.1.5", "0.1.5", false},
		{"0.1.4", "0.1.5", false},
		{"0.2", "0.10.0", false}, // 2 < 10 numerically
		{"0.10.0", "0.9.9", true},
		{"1.0", "1.0.1", false},
	}
	for _, c := range cases {
		if got := versionGreaterThan(c.a, c.b); got != c.want {
			t.Errorf("versionGreaterThan(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
