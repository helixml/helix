package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/stripe-go/v76"
)

func TestWalletUsesProQuotaTier(t *testing.T) {
	tests := []struct {
		name   string
		wallet *Wallet
		want   bool
	}{
		{name: "missing wallet"},
		{name: "pro override", wallet: &Wallet{PlanOverride: PlanOverridePro}, want: true},
		{name: "free override beats subscription", wallet: &Wallet{PlanOverride: PlanOverrideFree, StripeSubscriptionID: "sub_1", SubscriptionStatus: stripe.SubscriptionStatusActive}},
		{name: "active subscription", wallet: &Wallet{StripeSubscriptionID: "sub_1", SubscriptionStatus: stripe.SubscriptionStatusActive}, want: true},
		{name: "trialing subscription", wallet: &Wallet{StripeSubscriptionID: "sub_1", SubscriptionStatus: stripe.SubscriptionStatusTrialing}, want: true},
		{name: "status without subscription id", wallet: &Wallet{SubscriptionStatus: stripe.SubscriptionStatusActive}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.wallet.UsesProQuotaTier())
		})
	}
}
