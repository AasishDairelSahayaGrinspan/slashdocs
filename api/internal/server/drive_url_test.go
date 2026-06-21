// api/internal/server/drive_url_test.go
package server

import "testing"

func TestParseDriveFileRef(t *testing.T) {
	cases := []struct {
		name, in, wantID, wantRK string
	}{
		{"file-view", "https://drive.google.com/file/d/ABC123/view?usp=sharing", "ABC123", ""},
		{"file-view-resourcekey", "https://drive.google.com/file/d/ABC123/view?resourcekey=RK9", "ABC123", "RK9"},
		{"open-id", "https://drive.google.com/open?id=XYZ789", "XYZ789", ""},
		{"open-id-rk", "https://drive.google.com/open?id=XYZ789&resourcekey=RK1", "XYZ789", "RK1"},
		{"uc", "https://drive.google.com/uc?id=UC456", "UC456", ""},
		{"bare-id", "ABC123", "ABC123", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id, rk, err := parseDriveFileRef(c.in)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if id != c.wantID || rk != c.wantRK {
				t.Fatalf("got (%q,%q), want (%q,%q)", id, rk, c.wantID, c.wantRK)
			}
		})
	}
}

func TestParseDriveFileRef_Rejects(t *testing.T) {
	for _, in := range []string{
		"",
		"   ",
		"https://docs.google.com/document/d/DOCID/edit",
		"https://docs.google.com/spreadsheets/d/SID/edit",
		"https://docs.google.com/presentation/d/PID/edit",
		"https://example.com/not-a-drive-link",
	} {
		if _, _, err := parseDriveFileRef(in); err == nil {
			t.Fatalf("expected error for %q", in)
		}
	}
}
