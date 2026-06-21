package server

import "testing"

func TestParseDriveState(t *testing.T) {
	// parseDriveState receives the value Gin has already URL-decoded, i.e. raw JSON.
	raw := `{"ids":["FILE1"],"action":"open","userId":"112233"}`
	st, err := parseDriveState(raw)
	if err != nil {
		t.Fatal(err)
	}
	if st.Action != "open" || len(st.IDs) != 1 || st.IDs[0] != "FILE1" || st.UserID != "112233" {
		t.Fatalf("bad parse: %+v", st)
	}
}

func TestParseDriveState_ResourceKeys(t *testing.T) {
	raw := `{"ids":["F2"],"action":"open","userId":"u","resourceKeys":{"F2":"rk-1"}}`
	st, err := parseDriveState(raw)
	if err != nil {
		t.Fatal(err)
	}
	if rk := st.ResourceKeys["F2"]; rk != "rk-1" {
		t.Fatalf("resource key = %q, want rk-1", rk)
	}
}

func TestParseDriveState_Rejects(t *testing.T) {
	for _, raw := range []string{"", "not-json", `{"action":"open"}`} { // missing ids
		if _, err := parseDriveState(raw); err == nil {
			t.Fatalf("expected error for %q", raw)
		}
	}
}
