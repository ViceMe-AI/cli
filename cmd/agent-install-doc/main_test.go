package main

import "testing"

func TestStableVersionContract(t *testing.T) {
	for _, value := range []string{"0.1.0", "1.2.3", "12.0.99", "1.2.3-poc.1"} {
		if !exactVersion.MatchString(value) {
			t.Fatalf("exact version was rejected: %s", value)
		}
	}
	for _, value := range []string{"v1.2.3", "1.2", "1.2.3-beta.1", "1.2.3-poc.01", "01.2.3"} {
		if exactVersion.MatchString(value) {
			t.Fatalf("unsupported version was accepted: %s", value)
		}
	}
}
