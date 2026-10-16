package cloud

import "testing"

func TestRewardAdministrationPermissions(t *testing.T) {
	for _, role := range []string{"viewer", "support", "moderation", "operations"} {
		for _, path := range []string{"/v1/admin/rewards", "/v1/admin/rewards/funding", "/v1/admin/rewards/batches/id/confirm"} {
			for _, method := range []string{"GET", "POST"} {
				if adminAllowed(role, method, path) {
					t.Fatalf("%s can %s %s", role, method, path)
				}
			}
		}
	}
	for _, role := range []string{"finance", "owner"} {
		if !adminAllowed(role, "GET", "/v1/admin/rewards") || !adminAllowed(role, "POST", "/v1/admin/rewards/batches/id/confirm") {
			t.Fatalf("finance access denied %s", role)
		}
	}
}
