package stripe

import (
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/stretchr/testify/require"
	stripeapi "github.com/stripe/stripe-go/v76"
)

func TestValidateOrgSubscriptionPrice(t *testing.T) {
	t.Run("accepts the advertised monthly price", func(t *testing.T) {
		err := validateOrgSubscriptionPrice(&stripeapi.Price{
			ID:         "price_499",
			Currency:   stripeapi.CurrencyUSD,
			UnitAmount: 49900,
			Recurring: &stripeapi.PriceRecurring{
				Interval: stripeapi.PriceRecurringIntervalMonth,
			},
		}, config.Stripe{OrgPriceCents: 49900, OrgPriceCurrency: "usd", OrgPriceInterval: "month"})
		require.NoError(t, err)
	})

	t.Run("rejects a stale Stripe price", func(t *testing.T) {
		err := validateOrgSubscriptionPrice(&stripeapi.Price{
			ID:         "price_399",
			Currency:   stripeapi.CurrencyUSD,
			UnitAmount: 39900,
		}, config.Stripe{OrgPriceCents: 49900, OrgPriceCurrency: "usd", OrgPriceInterval: "month"})
		require.EqualError(t, err, "organization subscription price price_399 does not match configured 49900 usd/month")
	})
}

func TestCheckoutReturnURLsPreserveQuery(t *testing.T) {
	success, canceled, err := checkoutReturnURLs(
		"https://app.helix.ml",
		"/onboarding?org_id=org_1&created_org=true&step=provider&success=false&canceled=true&session_id=caller",
		"unused",
		"unused",
	)
	if err != nil {
		t.Fatal(err)
	}
	if success != "https://app.helix.ml/onboarding?created_org=true&org_id=org_1&step=provider&success=true&session_id={CHECKOUT_SESSION_ID}" {
		t.Fatalf("unexpected success URL: %s", success)
	}
	if canceled != "https://app.helix.ml/onboarding?created_org=true&org_id=org_1&step=provider&canceled=true" {
		t.Fatalf("unexpected cancel URL: %s", canceled)
	}
}

func TestCheckoutReturnURLsRejectUnsafePaths(t *testing.T) {
	for _, returnURL := range []string{
		"https://evil.example/onboarding",
		"//evil.example/onboarding",
		`/\evil.example/onboarding`,
		"onboarding",
		"/%zz",
		"/onboarding#success-hidden",
	} {
		t.Run(returnURL, func(t *testing.T) {
			_, _, err := checkoutReturnURLs("https://app.helix.ml", returnURL, "unused", "unused")
			if err == nil {
				t.Fatalf("expected %q to be rejected", returnURL)
			}
		})
	}
}

func TestCheckoutFlowsRejectUnsafeReturnURL(t *testing.T) {
	client := NewStripe(config.Stripe{
		SecretKey:            "sk_test",
		WebhookSigningSecret: "whsec_test",
	}, nil)

	if _, err := client.GetTopUpSessionURL(TopUpSessionParams{ReturnURL: "//evil.example"}); err == nil {
		t.Fatal("top-up checkout accepted an unsafe return URL")
	}
	if _, err := client.GetCheckoutSessionURL(SubscriptionSessionParams{ReturnURL: "//evil.example"}); err == nil {
		t.Fatal("subscription checkout accepted an unsafe return URL")
	}
}
