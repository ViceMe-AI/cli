package main

import "testing"

func TestStableVersionContract(t *testing.T) {
	for _, value := range []string{"0.1.0", "1.2.3", "12.0.99"} {
		if !stableVersion.MatchString(value) {
			t.Fatalf("stable version was rejected: %s", value)
		}
	}
	for _, value := range []string{"v1.2.3", "1.2", "1.2.3-beta.1", "01.2.3"} {
		if stableVersion.MatchString(value) {
			t.Fatalf("non-stable version was accepted: %s", value)
		}
	}
}
