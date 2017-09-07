package kingpin

import (
	"net/url"
	"os"
	"regexp"

	"github.com/alecthomas/units"
)

var (
	envVarValuesSeparator = "\r?\n"
	envVarValuesTrimmer   = regexp.MustCompile(envVarValuesSeparator + "$")
	envVarValuesSplitter  = regexp.MustCompile(envVarValuesSeparator)
)

type Settings interface {
	SetValue(value Value)
}

// A Clause represents a flag or an argument passed by the user.
type Clause struct {
	actionMixin
	completionsMixin

	name               string
	shorthand          rune
	help               string
	placeholder        string
	hidden             bool
	defaultValues      []string
	value              Value
	required           bool
	envar              string
	noEnvar            bool
	hintActions        []HintAction
	builtinHintActions []HintAction
}

func NewClause(name, help string) *Clause {
	return &Clause{
		name: name,
		help: help,
	}
}

func (c *Clause) consumesRemainder() bool {
	if r, ok := c.value.(cumulativeValue); ok {
		return r.IsCumulative()
	}
	return false
}

func (c *Clause) init() error {
	if c.required && len(c.defaultValues) > 0 {
		return TError("required flag '--{{.Arg0}}' with default value that will never be used", V{"Arg0": c.name})
	}
	if c.value == nil {
		return TError("no type defined for --{{.Arg0}} (eg. .String())", V{"Arg0": c.name})
	}
	if v, ok := c.value.(cumulativeValue); (!ok || !v.IsCumulative()) && len(c.defaultValues) > 1 {
		return TError("invalid default for '--{{.Arg0}}', expecting single value", V{"Arg0": c.name})
	}
	return nil
}

// UsageAction adds a PreAction() that will display the given UsageContext.
func (c *Clause) UsageAction(context *UsageContext) *Clause {
	c.PreAction(func(a *Application, e *ParseElement, c *ParseContext) error {
		a.UsageForContextWithTemplate(context, c)
		a.terminate(0)
		return nil
	})
	return c
}

func (c *Clause) UsageActionTemplate(template string) *Clause {
	return c.UsageAction(&UsageContext{Template: template})
}

func (c *Clause) Action(action Action) *Clause {
	c.actions = append(c.actions, action)
	return c
}

// PreAction callback executed
func (c *Clause) PreAction(action Action) *Clause {
	c.preActions = append(c.preActions, action)
	return c
}

// HintAction registers a HintAction (function) for the flag to provide completions
func (c *Clause) HintAction(action HintAction) *Clause {
	c.addHintAction(action)
	return c
}

func (c *Clause) addHintAction(action HintAction) {
	c.hintActions = append(c.hintActions, action)
}

// Allow adding of HintActions which are added internally, ie, EnumVar
func (c *Clause) addHintActionBuiltin(action HintAction) {
	c.builtinHintActions = append(c.builtinHintActions, action)
}

func (c *Clause) resolveCompletions() []string {
	var hints []string

	options := c.builtinHintActions
	if len(c.hintActions) > 0 {
		// User specified their own hintActions. Use those instead.
		options = c.hintActions
	}

	for _, hintAction := range options {
		hints = append(hints, hintAction()...)
	}
	return hints
}

// HintOptions registers any number of options for the flag to provide completions
func (c *Clause) HintOptions(options ...string) *Clause {
	c.addHintAction(func() []string {
		return options
	})
	return c
}

// Default values for this flag. They *must* be parseable by the value of the flag.
func (c *Clause) Default(values ...string) *Clause {
	c.defaultValues = values
	return c
}

// Envar overrides the default value(s) for a flag from an environment variable,
// if it is set. Several default values can be provided by using new lines to
// separate them.
func (c *Clause) Envar(name string) *Clause {
	c.envar = name
	c.noEnvar = false
	return c
}

// NoEnvar forces environment variable defaults to be disabled for this flag.
// Most useful in conjunction with app.DefaultEnvars().
func (c *Clause) NoEnvar() *Clause {
	c.envar = ""
	c.noEnvar = true
	return c
}

// PlaceHolder sets the place-holder string used for flag values in the help. The
// default behaviour is to use the value provided by Default() if provided,
// then fall back on the capitalized flag name.
func (c *Clause) PlaceHolder(placeholder string) *Clause {
	c.placeholder = placeholder
	return c
}

// Hidden hides a flag from usage but still allows it to be used.
func (c *Clause) Hidden() *Clause {
	c.hidden = true
	return c
}

// Required makes the flag required. You can not provide a Default() value to a Required() flag.
func (c *Clause) Required() *Clause {
	c.required = true
	return c
}

// Short sets the short flag name.
func (c *Clause) Short(name rune) *Clause {
	c.shorthand = name
	return c
}

func (c *Clause) needsValue() bool {
	haveDefault := len(c.defaultValues) > 0
	return c.required && !(haveDefault || c.HasEnvarValue())
}

func (c *Clause) reset() {
	if c, ok := c.value.(cumulativeValue); ok {
		c.Reset()
	}
}

func (c *Clause) setDefault() error {
	if c.HasEnvarValue() {
		c.reset()
		if v, ok := c.value.(cumulativeValue); !ok || !v.IsCumulative() {
			// Use the value as-is
			return c.value.Set(c.GetEnvarValue())
		}
		for _, value := range c.GetSplitEnvarValue() {
			if err := c.value.Set(value); err != nil {
				return err
			}
		}
		return nil
	} else if len(c.defaultValues) > 0 {
		c.reset()
		for _, defaultValue := range c.defaultValues {
			if err := c.value.Set(defaultValue); err != nil {
				return err
			}
		}
		return nil
	}

	return nil
}

func (c *Clause) HasEnvarValue() bool {
	return c.GetEnvarValue() != ""
}

func (c *Clause) GetEnvarValue() string {
	if c.noEnvar || c.envar == "" {
		return ""
	}
	return os.Getenv(c.envar)
}

func (c *Clause) GetSplitEnvarValue() []string {
	values := make([]string, 0)

	envarValue := c.GetEnvarValue()
	if envarValue == "" {
		return values
	}

	// Split by new line to extract multiple values, if any.
	trimmed := envVarValuesTrimmer.ReplaceAllString(envarValue, "")
	values = append(values, envVarValuesSplitter.Split(trimmed, -1)...)
	return values
}

func (c *Clause) SetValue(value Value) {
	c.value = value
	c.setDefault()
}

// StringMap provides key=value parsing into a map.
func (c *Clause) StringMap() (target *map[string]string) {
	target = &(map[string]string{})
	c.StringMapVar(target)
	return
}

// Bytes parses numeric byte units. eg. 1.5KB
func (c *Clause) Bytes() (target *units.Base2Bytes) {
	target = new(units.Base2Bytes)
	c.BytesVar(target)
	return
}

// ExistingFile sets the parser to one that requires and returns an existing file.
func (c *Clause) ExistingFile() (target *string) {
	target = new(string)
	c.ExistingFileVar(target)
	return
}

// ExistingDir sets the parser to one that requires and returns an existing directory.
func (c *Clause) ExistingDir() (target *string) {
	target = new(string)
	c.ExistingDirVar(target)
	return
}

// ExistingFileOrDir sets the parser to one that requires and returns an existing file OR directory.
func (c *Clause) ExistingFileOrDir() (target *string) {
	target = new(string)
	c.ExistingFileOrDirVar(target)
	return
}

// URL provides a valid, parsed url.URL.
func (c *Clause) URL() (target **url.URL) {
	target = new(*url.URL)
	c.URLVar(target)
	return
}

// StringMap provides key=value parsing into a map.
func (c *Clause) StringMapVar(target *map[string]string) {
	c.SetValue(newStringMapValue(target))
}

// Float sets the parser to a float64 parser.
func (c *Clause) Float() (target *float64) {
	return c.Float64()
}

// Float sets the parser to a float64 parser.
func (c *Clause) FloatVar(target *float64) {
	c.Float64Var(target)
}

// BytesVar parses numeric byte units. eg. 1.5KB
func (c *Clause) BytesVar(target *units.Base2Bytes) {
	c.SetValue(newBytesValue(target))
}

// ExistingFile sets the parser to one that requires and returns an existing file.
func (c *Clause) ExistingFileVar(target *string) {
	c.SetValue(newExistingFileValue(target))
}

// ExistingDir sets the parser to one that requires and returns an existing directory.
func (c *Clause) ExistingDirVar(target *string) {
	c.SetValue(newExistingDirValue(target))
}

// ExistingDir sets the parser to one that requires and returns an existing directory.
func (c *Clause) ExistingFileOrDirVar(target *string) {
	c.SetValue(newExistingFileOrDirValue(target))
}

// URL provides a valid, parsed url.URL.
func (c *Clause) URLVar(target **url.URL) {
	c.SetValue(newURLValue(target))
}

// URLList provides a parsed list of url.URL values.
func (c *Clause) URLList() (target *[]*url.URL) {
	target = new([]*url.URL)
	c.URLListVar(target)
	return
}

// URLListVar provides a parsed list of url.URL values.
func (c *Clause) URLListVar(target *[]*url.URL) {
	c.SetValue(newURLListValue(target))
}

// Enum allows a value from a set of options.
func (c *Clause) Enum(options ...string) (target *string) {
	target = new(string)
	c.EnumVar(target, options...)
	return
}

// EnumVar allows a value from a set of options.
func (c *Clause) EnumVar(target *string, options ...string) {
	c.addHintActionBuiltin(func() []string { return options })
	c.SetValue(newEnumFlag(target, options...))
}

// Enums allows a set of values from a set of options.
func (c *Clause) Enums(options ...string) (target *[]string) {
	target = new([]string)
	c.EnumsVar(target, options...)
	return
}

// EnumVar allows a value from a set of options.
func (c *Clause) EnumsVar(target *[]string, options ...string) {
	c.SetValue(newEnumsFlag(target, options...))
}

// A Counter increments a number each time it is encountered.
func (c *Clause) Counter() (target *int) {
	target = new(int)
	c.CounterVar(target)
	return
}

func (c *Clause) CounterVar(target *int) {
	c.SetValue(newCounterValue(target))
}
