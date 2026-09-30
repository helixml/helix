package types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeBotInstanceIdleTimeout(t *testing.T) {
	for _, tc := range []struct {
		seconds int
		wantErr bool
	}{
		{seconds: 0, wantErr: false},      // inherit
		{seconds: 300, wantErr: false},    // min
		{seconds: 3600, wantErr: false},   // 1h
		{seconds: 604800, wantErr: false}, // max
		{seconds: 299, wantErr: true},     // below min
		{seconds: 604801, wantErr: true},  // above max
		{seconds: -3600, wantErr: true},   // negative
	} {
		_, err := NormalizeBotInstanceIdleTimeout(tc.seconds)
		if tc.wantErr {
			require.Error(t, err, "seconds=%d", tc.seconds)
		} else {
			require.NoError(t, err, "seconds=%d", tc.seconds)
		}
	}
}

func TestIdleSecondsUnmarshalJSON(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int
	}{
		{in: `{"idle_timeout_seconds": 900}`, want: 900},
		{in: `{"idle_timeout_seconds": "900"}`, want: 900}, // numeric string
		{in: `{"idle_timeout_seconds": null}`, want: 0},    // null → inherit
		{in: `{"idle_timeout_seconds": "soon"}`, want: 0},  // garbage → inherit
		{in: `{"idle_timeout_seconds": []}`, want: 0},      // wrong shape → inherit
		{in: `{"idle_timeout_seconds": 1.5}`, want: 1},     // fractional truncates
		{in: `{}`, want: 0},
	} {
		var profile BotInstanceProfile
		require.NoError(t, json.Unmarshal([]byte(tc.in), &profile), "in=%s", tc.in)
		require.Equal(t, tc.want, int(profile.IdleTimeoutSeconds), "in=%s", tc.in)
	}
}

func TestIdleSecondsMarshalJSON(t *testing.T) {
	bts, err := json.Marshal(BotInstanceProfile{IdleTimeoutSeconds: 900})
	require.NoError(t, err)
	require.JSONEq(t, `{"idle_timeout_seconds":900,"mcp_servers":null,"tools":null}`, string(bts))
}

func TestBotInstanceProfileValidateIdleTimeout(t *testing.T) {
	require.Error(t, (BotInstanceProfile{IdleTimeoutSeconds: 60}).Validate())
	require.NoError(t, (BotInstanceProfile{IdleTimeoutSeconds: 0}).Validate())
	require.NoError(t, (BotInstanceProfile{IdleTimeoutSeconds: 900}).Validate())
}
