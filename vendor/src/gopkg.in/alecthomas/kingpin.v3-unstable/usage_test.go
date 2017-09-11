package kingpin

import (
	"bytes"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatTwoColumns(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	formatTwoColumns(buf, 2, 2, 20, [][2]string{
		{"--hello", "Hello world help with something that is cool."},
	})
	expected := `  --hello  Hello
           world
           help with
           something
           that is
           cool.
`
	assert.Equal(t, expected, buf.String())
}

func TestRequiredSubcommandsUsage(t *testing.T) {
	var buf bytes.Buffer

	a := New("test", "Test").Writers(&buf, ioutil.Discard).Terminate(nil)
	c0 := a.Command("c0", "c0info")
	c0.Command("c1", "c1info")
	_, err := a.Parse(nil)
	assert.Error(t, err)

	expectedStr := `
usage: test [<flags>] <command>

Test


Flags:
  --help  Show context-sensitive help.

Commands:
  help [<command> ...]
    Show help.


  c0 c1
    c1info


`
	assert.Equal(t, expectedStr, buf.String())
}

func TestOptionalSubcommandsUsage(t *testing.T) {
	var buf bytes.Buffer

	a := New("test", "Test").Writers(&buf, ioutil.Discard).Terminate(nil)
	c0 := a.Command("c0", "c0info").OptionalSubcommands()
	c0.Command("c1", "c1info")
	_, err := a.Parse(nil)
	assert.Error(t, err)

	expectedStr := `
usage: test [<flags>] <command>

Test


Flags:
  --help  Show context-sensitive help.

Commands:
  help [<command> ...]
    Show help.


  c0
    c0info


  c0 c1
    c1info


`
	assert.Equal(t, expectedStr, buf.String())
}

func TestFormatTwoColumnsWide(t *testing.T) {
	samples := [][2]string{
		{strings.Repeat("x", 29), "29 chars"},
		{strings.Repeat("x", 30), "30 chars"}}
	buf := bytes.NewBuffer(nil)
	formatTwoColumns(buf, 0, 0, 200, samples)
	expected := `xxxxxxxxxxxxxxxxxxxxxxxxxxxxx29 chars
xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
                             30 chars
`
	assert.Equal(t, expected, buf.String())
}

func TestHiddenCommand(t *testing.T) {
	templates := []struct{ name, template string }{
		{"default", DefaultUsageTemplate},
		{"Compact", CompactUsageTemplate},
		{"Man", ManPageTemplate},
	}

	var buf bytes.Buffer

	a := New("test", "Test").Writers(&buf, ioutil.Discard).Terminate(nil)
	a.Command("visible", "visible")
	a.Command("hidden", "hidden").Hidden()

	for _, tp := range templates {
		buf.Reset()
		a.UsageTemplate(tp.template)
		a.Parse(nil)
		// a.Parse([]string{"--help"})
		usage := buf.String()
		t.Logf("Usage for %s is:\n%s\n", tp.name, usage)

		assert.NotContains(t, usage, "hidden")
		assert.Contains(t, usage, "visible")
	}
}

func TestIssue169MultipleUsageCorruption(t *testing.T) {
	buf := &bytes.Buffer{}
	app := newTestApp()
	cmd := app.Command("cmd", "")
	cmd.Flag("flag", "").Default("false").Bool()
	app.Writers(buf, ioutil.Discard)
	_, err := app.Parse([]string{"help", "cmd"})
	assert.NoError(t, err)
	expected := buf.String()

	buf.Reset()
	_, err = app.Parse([]string{"help"})
	assert.NoError(t, err)
	actual := buf.String()

	assert.NotEqual(t, expected, actual)
}
