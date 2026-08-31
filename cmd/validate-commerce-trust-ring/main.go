package main

import (
	"fmt"
	"os"

	"github.com/ViceMe-AI/cli/internal/commerceartifact"
)

func main() {
	ring := os.Getenv("COMMERCE_SKILL_TRUST_KEYS")
	keys, err := commerceartifact.ParseTrustRing(ring)
	if err != nil || len(keys) == 0 {
		fmt.Fprintln(os.Stderr, "COMMERCE_SKILL_TRUST_RING_INVALID")
		os.Exit(1)
	}
}
