package kingpin

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func cmdElem() *ParseElement {
	return &ParseElement{
		OneOf: OneOfClause{
			Cmd: &CmdClause{},
		},
	}
}

func TestParseContextPush(t *testing.T) {
	c := tokenize([]string{"foo", "bar"}, false)
	a := c.Next()
	assert.Equal(t, TokenArg, a.Type)
	b := c.Next()
	assert.Equal(t, TokenArg, b.Type)
	c.Push(b)
	c.Push(a)
	a = c.Next()
	assert.Equal(t, "foo", a.Value)
	b = c.Next()
	assert.Equal(t, "bar", b.Value)
}

func TestLastCmd(t *testing.T) {
	e := cmdElem()
	pc := &ParseContext{
		Elements: []*ParseElement{e, cmdElem(), cmdElem()},
	}
	assert.Equal(t, false, pc.LastCmd(e))

	pc = &ParseContext{
		Elements: []*ParseElement{cmdElem(), e, cmdElem()},
	}
	assert.Equal(t, false, pc.LastCmd(e))

	pc = &ParseContext{
		Elements: []*ParseElement{cmdElem(), cmdElem(), e},
	}
	assert.Equal(t, true, pc.LastCmd(e))
}
