package server

import "testing"

func TestMCPRequestLimitPreservesHTMLEscapingHeadroom(t *testing.T) {
	// A JSON encoder may represent one input byte as a six-byte escape. Leave
	// another MiB for the JSON-RPC request and tool argument envelope.
	const minimum = int64(maxHTMLBytes*6 + (1 << 20))
	if maxMCPRequestBytes < minimum {
		t.Fatalf("MCP request limit = %d, want at least %d for a maximally escaped HTML payload", maxMCPRequestBytes, minimum)
	}
}
