package kingpin

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFlagsStruct(t *testing.T) {
	type MyFlags struct {
		Debug bool     `help:"Enable debug mode."`
		URL   string   `help:"URL to connect to." default:"localhost:80"`
		Names []string `help:"Names of things."`
	}
	a := newTestApp()
	actual := &MyFlags{}
	err := a.Struct(actual)
	assert.NoError(t, err)
	assert.NotNil(t, a.flagGroup.long["debug"])
	assert.NotNil(t, a.flagGroup.long["url"])

	*actual = MyFlags{}
	a.Parse([]string{})
	assert.Equal(t, &MyFlags{URL: "localhost:80"}, actual)

	*actual = MyFlags{}
	a.Parse([]string{"--debug"})
	assert.Equal(t, &MyFlags{Debug: true, URL: "localhost:80"}, actual)

	*actual = MyFlags{}
	a.Parse([]string{"--url=w3.org"})
	assert.Equal(t, &MyFlags{URL: "w3.org"}, actual)

	*actual = MyFlags{}
	a.Parse([]string{"--names=alec", "--names=bob"})
	assert.Equal(t, &MyFlags{URL: "localhost:80", Names: []string{"alec", "bob"}}, actual)

	type RequiredFlag struct {
		Flag bool `help:"A flag." required:"true"`
	}

	a = newTestApp()
	rflags := &RequiredFlag{}
	err = a.Struct(rflags)
	assert.NoError(t, err)
	_, err = a.Parse([]string{})
	assert.Error(t, err)
	_, err = a.Parse([]string{"--flag"})
	assert.NoError(t, err)
	assert.Equal(t, &RequiredFlag{Flag: true}, rflags)

	type DurationFlag struct {
		Elapsed time.Duration `help:"Elapsed time."`
	}

	a = newTestApp()
	dflag := &DurationFlag{}
	err = a.Struct(dflag)
	assert.NoError(t, err)
	_, err = a.Parse([]string{"--elapsed=5s"})
	assert.NoError(t, err)
	assert.Equal(t, 5*time.Second, dflag.Elapsed)
}

func TestNestedStruct(t *testing.T) {
	type NestedFlags struct {
		URL string `help:"URL to connect to." default:"localhost:80"`
	}

	type MyFlags struct {
		NestedFlags
		Debug bool `help:"Enable debug mode"`
	}

	a := newTestApp()
	actual := &MyFlags{}
	err := a.Struct(actual)
	assert.NoError(t, err)

	assert.NotNil(t, a.GetFlag("debug"))
	assert.NotNil(t, a.GetFlag("url"))

	_, err = a.Parse([]string{"--debug", "--url=foobar"})
	assert.NoError(t, err)
	assert.True(t, actual.Debug)
	assert.Equal(t, "foobar", actual.URL)
}

func TestStructHierarchy(t *testing.T) {
	type App struct {
		Login struct {
			Username string `arg:"true" required:"true"`
		}
		Debug bool
	}
	a := newTestApp()
	actual := &App{}
	err := a.Struct(actual)
	assert.NoError(t, err)
	expected := &App{
		Login: struct {
			Username string `arg:"true" required:"true"`
		}{
			Username: "alec",
		},
		Debug: true,
	}
	_, err = a.Parse([]string{"--debug", "login", "alec"})
	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestStructEnum(t *testing.T) {
	type App struct {
		Enum  string   `enum:"one,two"`
		Enums []string `enum:"one,two"`
	}
	actual := &App{}
	a := newTestApp()
	a.Struct(actual)
	_, err := a.Parse([]string{"--enum=three"})
	assert.Error(t, err)
	_, err = a.Parse([]string{"--enums=three"})
	assert.Error(t, err)
	_, err = a.Parse([]string{"--enum=one", "--enums=one", "--enums=two"})
	assert.NoError(t, err)
	assert.Equal(t, &App{Enum: "one", Enums: []string{"one", "two"}}, actual)
}

type onActionStructTest struct {
	Debug bool

	called int
}

func (o *onActionStructTest) OnDebug(app *Application, element *ParseElement, context *ParseContext) error {
	o.called++
	return nil
}

func TestStructOnAction(t *testing.T) {
	actual := &onActionStructTest{}
	var _ Action = actual.OnDebug
	a := newTestApp()
	err := a.Struct(actual)
	assert.NoError(t, err)
	_, err = a.Parse([]string{"--debug"})
	assert.NoError(t, err)
	assert.Equal(t, &onActionStructTest{Debug: true, called: 1}, actual)
}
