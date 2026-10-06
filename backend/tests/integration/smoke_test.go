package integration

import "testing"

func TestHealth(t *testing.T) {
	r := call(t, "GET", "/health")
	expect(t, r, 200, "health")
}
