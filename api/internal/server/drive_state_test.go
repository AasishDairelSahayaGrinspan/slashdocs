package server

import "testing"

func TestParseDriveState(t *testing.T) {
	raw := `%7B%22ids%22%3A%5B%22FILE1%22%5D%2C%22action%22%3A%22open%22%2C%22userId%22%3A%22112233%22%7D`
	st, err := parseDriveState(raw)
	if err != nil {
		t.Fatal(err)
	}
	if st.Action != "open" || len(st.IDs) != 1 || st.IDs[0] != "FILE1" || st.UserID != "112233" {
		t.Fatalf("bad parse: %+v", st)
	}
}

func TestParseDriveState_ResourceKeys(t *testing.T) {
	// {"ids":["F2"],"action":"open","userId":"u","resourceKeys":{"F2":"rk-1"}}
	raw := `%7B%22ids%22%3A%5B%22F2%22%5D%2C%22action%22%3A%22open%22%2C%22userId%22%3A%22u%22%2C%22resourceKeys%22%3A%7B%22F2%22%3A%22rk-1%22%7D%7D`
	st, err := parseDriveState(raw)
	if err != nil {
		t.Fatal(err)
	}
	if rk := st.ResourceKeys["F2"]; rk != "rk-1" {
		t.Fatalf("resource key = %q, want rk-1", rk)
	}
}

func TestParseDriveState_Rejects(t *testing.T) {
	for _, raw := range []string{"", "not-json", `%7B%22action%22%3A%22open%22%7D`} { // missing ids
		if _, err := parseDriveState(raw); err == nil {
			t.Fatalf("expected error for %q", raw)
		}
	}
}
