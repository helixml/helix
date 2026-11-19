package types

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestTruncateRunes(t *testing.T) {
	t.Run("short value is unchanged", func(t *testing.T) {
		require.Equal(t, "Hypothesis bot — luke", TruncateRunes("Hypothesis bot — luke", 128, true))
	})

	t.Run("exactly max is unchanged", func(t *testing.T) {
		s := strings.Repeat("a", 128)
		require.Equal(t, s, TruncateRunes(s, 128, true))
	})

	t.Run("long value ends in ellipsis at max runes", func(t *testing.T) {
		got := TruncateRunes(strings.Repeat("a", 600), SandboxNameMaxLen, true)
		require.Equal(t, SandboxNameMaxLen, utf8.RuneCountInString(got))
		require.True(t, strings.HasSuffix(got, "…"))
	})

	t.Run("without ellipsis keeps max runes", func(t *testing.T) {
		got := TruncateRunes(strings.Repeat("b", 200), SandboxOrgBotIDMaxLen, false)
		require.Equal(t, strings.Repeat("b", SandboxOrgBotIDMaxLen), got)
	})

	t.Run("multi-byte runes straddling the limit stay valid", func(t *testing.T) {
		for _, filler := range []string{"🚀", "漢", "é"} {
			s := strings.Repeat("x", 9) + strings.Repeat(filler, 20)
			for _, max := range []int{9, 10, 11, 12} {
				got := TruncateRunes(s, max, true)
				require.True(t, utf8.ValidString(got), "filler %q max %d", filler, max)
				require.Equal(t, max, utf8.RuneCountInString(got))
				got = TruncateRunes(s, max, false)
				require.True(t, utf8.ValidString(got))
				require.Equal(t, max, utf8.RuneCountInString(got))
			}
		}
	})

	t.Run("invalid UTF-8 input yields valid output", func(t *testing.T) {
		got := TruncateRunes("ab\xffcd", 3, false)
		require.True(t, utf8.ValidString(got))
	})
}
