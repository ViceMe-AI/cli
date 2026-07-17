package semver

import "testing"

func TestCompare(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		left  string
		right string
		want  int
	}{
		{"0.1.0", "0.1.0", 0},
		{"0.2.0", "0.1.9", 1},
		{"1.0.0-beta.2", "1.0.0-beta.11", -1},
		{"1.0.0", "1.0.0-rc.1", 1},
	} {
		got, err := Compare(test.left, test.right)
		if err != nil {
			t.Fatalf("Compare(%q, %q): %v", test.left, test.right, err)
		}
		if got != test.want {
			t.Errorf("Compare(%q, %q)=%d, want %d", test.left, test.right, got, test.want)
		}
	}
}

func TestSatisfies(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		version    string
		constraint string
		want       bool
	}{
		{"0.1.0", ">=0.1.0 <0.2.0", true},
		{"0.1.9", ">=0.1.0 <0.2.0", true},
		{"0.2.0", ">=0.1.0 <0.2.0", false},
		{"0.1.0-rc.1", ">=0.1.0", false},
	} {
		got, err := Satisfies(test.version, test.constraint)
		if err != nil {
			t.Fatalf("Satisfies(%q, %q): %v", test.version, test.constraint, err)
		}
		if got != test.want {
			t.Errorf("Satisfies(%q, %q)=%t, want %t", test.version, test.constraint, got, test.want)
		}
	}
}
