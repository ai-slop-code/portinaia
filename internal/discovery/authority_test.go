package discovery

import "testing"

func TestAuthorityRequiresSuccessfulOutcomeAndAllowsEmptySuccess(t *testing.T) {
	if !(CollectorOutcome{Status: "success"}).Authoritative() {
		t.Fatal("explicit successful empty collection must be authoritative")
	}
	for _, status := range []string{"failed", "", "unknown"} {
		if (CollectorOutcome{Status: status}).Authoritative() {
			t.Errorf("status %q must not establish authority", status)
		}
	}
}
