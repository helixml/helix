package yellowdog

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLive_ListNamespaces hits the real YellowDog platform at
// https://portal.yellowdog.co/api/namespaces using credentials from
// the environment.
//
// Gated by YD_LIVE_TEST=1 so it never runs in CI. To run locally:
//
//	cd ~/helix/yellowdog-poc && set -a && source .env && set +a
//	cd /path/to/helix/api
//	YD_LIVE_TEST=1 go test ./pkg/yellowdog/ -run TestLive -v
//
// What this asserts:
//   - Auth header construction matches the platform's expectation.
//   - JSON decoding round-trips against real responses (the field
//     shape we modelled is the field shape the API returns).
//   - End-to-end plumbing (no proxy / TLS / DNS surprises).
//
// What this does NOT assert: any particular namespace contents - the
// test only requires the response to be a well-formed Page[Namespace]
// with at least zero items.
func TestLive_ListNamespaces(t *testing.T) {
	if os.Getenv("YD_LIVE_TEST") != "1" {
		t.Skip("set YD_LIVE_TEST=1 to enable; see test comment for instructions")
	}
	keyID := os.Getenv("YD_KEY")
	secret := os.Getenv("YD_SECRET")
	if keyID == "" || secret == "" {
		t.Fatal("YD_LIVE_TEST=1 but YD_KEY and/or YD_SECRET not set in environment")
	}

	c, err := NewClient(Credentials{KeyID: keyID, Secret: secret})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	page, err := c.ListNamespaces(ctx)
	if err != nil {
		t.Fatalf("ListNamespaces: %v", err)
	}
	t.Logf("live API returned %d namespace(s)", len(page.Items))
	for _, ns := range page.Items {
		// Sanity: at minimum the platform always assigns a non-empty
		// ID and a human-readable namespace name to each row.
		if ns.ID == "" || ns.Namespace == "" {
			t.Errorf("namespace row missing ID or Namespace: %+v", ns)
		}
		t.Logf("  id=%s namespace=%q deletable=%v", ns.ID, ns.Namespace, ns.Deletable)
	}
}
