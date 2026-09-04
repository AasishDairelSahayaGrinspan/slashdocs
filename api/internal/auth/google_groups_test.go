package auth

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGoogleGroupMembershipFieldsAreInternalOnly(t *testing.T) {
	p := Principal{
		Type:                       PrincipalUser,
		ID:                         "usr_1",
		Email:                      "member@example.com",
		GoogleGroupIDs:             []string{"groups/private-membership"},
		GoogleGroupMembershipState: GoogleGroupMembershipUnavailable,
	}
	body, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(body)
	for _, forbidden := range []string{"groups/private-membership", "GoogleGroupIDs", "google_group_membership", GoogleGroupMembershipUnavailable} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("serialized principal leaked internal group state: %s", serialized)
		}
	}
}
