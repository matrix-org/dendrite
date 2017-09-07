package kingpin

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFlagPreAction(t *testing.T) {
	a := newTestApp()
	actual := ""
	flag := a.Flag("flag", "").PreAction(func(_ *Application, e *ParseElement, c *ParseContext) error {
		actual = *e.Value
		return nil
	}).Bool()

	_, err := a.Parse([]string{})
	require.NoError(t, err)
	require.Equal(t, "", actual)
	require.False(t, *flag)

	_, err = a.Parse([]string{"--flag"})
	require.NoError(t, err)
	require.Equal(t, "true", actual)
	require.True(t, *flag)

	_, err = a.Parse([]string{"--no-flag"})
	require.NoError(t, err)
	require.Equal(t, "false", actual)
	require.False(t, *flag)
}

func TestFlagAction(t *testing.T) {
	a := newTestApp()
	actual := ""
	flag := a.Flag("flag", "").PreAction(func(_ *Application, e *ParseElement, c *ParseContext) error {
		actual = *e.Value
		return nil
	}).Bool()

	_, err := a.Parse([]string{})
	require.NoError(t, err)
	require.Equal(t, "", actual)
	require.False(t, *flag)

	_, err = a.Parse([]string{"--flag"})
	require.NoError(t, err)
	require.Equal(t, "true", actual)
	require.True(t, *flag)

	_, err = a.Parse([]string{"--no-flag"})
	require.NoError(t, err)
	require.Equal(t, "false", actual)
	require.False(t, *flag)
}
