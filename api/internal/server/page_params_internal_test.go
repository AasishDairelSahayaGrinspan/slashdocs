package server

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPageParams(t *testing.T) {
	tests := []struct {
		name       string
		rawQuery   string
		wantLimit  int32
		wantOffset int32
		wantErr    bool
	}{
		{"defaults when absent", "", searchPageDefault, 0, false},
		{"explicit limit", "limit=10", 10, 0, false},
		{"explicit offset", "offset=50", searchPageDefault, 50, false},
		{"limit and offset", "limit=100&offset=200", 100, 200, false},
		{"offset zero is valid", "offset=0", searchPageDefault, 0, false},
		{"limit clamped to max", "limit=100000", searchPageMax, 0, false},
		{"blank values fall back to defaults", "limit=&offset=", searchPageDefault, 0, false},
		// Rejected rather than silently ignored: a client sending these has a bug, and
		// quietly serving page 1 hides it.
		{"non-numeric limit", "limit=abc", 0, 0, true},
		{"zero limit", "limit=0", 0, 0, true},
		{"negative limit", "limit=-5", 0, 0, true},
		{"negative offset", "offset=-1", 0, 0, true},
		{"non-numeric offset", "offset=xyz", 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "/v1/documents?"+tt.rawQuery, nil)

			limit, offset, err := pageParams(c)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %q, got limit=%d offset=%d", tt.rawQuery, limit, offset)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.rawQuery, err)
			}
			if limit != tt.wantLimit {
				t.Errorf("limit = %d, want %d", limit, tt.wantLimit)
			}
			if offset != tt.wantOffset {
				t.Errorf("offset = %d, want %d", offset, tt.wantOffset)
			}
		})
	}
}
