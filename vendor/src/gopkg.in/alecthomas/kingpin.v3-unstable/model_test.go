package kingpin

import (
	"fmt"
	"reflect"

	"github.com/stretchr/testify/assert"

	"testing"
)

func TestFindModel(t *testing.T) {
	app := newTestApp()
	cmd := app.Command("cmd", "").Command("cmd2", "")
	model := app.Model()
	cmdModel := model.FindModelForCommand(cmd)
	assert.NotNil(t, cmdModel)
	assert.Equal(t, "cmd2", cmdModel.Name)
}

func TestFullCommand(t *testing.T) {
	app := newTestApp()
	cmd := app.Command("cmd", "").Command("cmd2", "")
	model := app.Model()
	cmdModel := model.FindModelForCommand(cmd)
	assert.Equal(t, "cmd cmd2", cmdModel.FullCommand())
}

func TestCmdSummary(t *testing.T) {
	app := newTestApp()
	cmd := app.Command("cmd", "")
	cmd.Flag("flag", "").Required().String()
	cmd = cmd.Command("cmd2", "")
	cmd.Arg("arg", "").Required().String()
	model := app.Model()
	cmdModel := model.FindModelForCommand(cmd)
	assert.Equal(t, "cmd --flag=FLAG cmd2 <arg>", cmdModel.CmdSummary())
}

func TestModelValue(t *testing.T) {
	app := newTestApp()
	value := app.Flag("test", "").Bool()
	*value = true
	model := app.Model()
	flag := model.FlagByName("test")
	fmt.Println(reflect.TypeOf(flag.Value))
	assert.Equal(t, "true", flag.Value.String())
}
