package macaroon

import (
	"strconv"
	"strings"
	"unicode"

	gc "gopkg.in/check.v1"
)

type packetV1Suite struct{}

var _ = gc.Suite(&packetV1Suite{})

func (*packetV1Suite) TestAppendPacket(c *gc.C) {
	data, ok := appendPacketV1(nil, "field", []byte("some data"))
	c.Assert(ok, gc.Equals, true)
	c.Assert(string(data), gc.Equals, "0014field some data\n")

	data, ok = appendPacketV1(data, "otherfield", []byte("more and more data"))
	c.Assert(ok, gc.Equals, true)
	c.Assert(string(data), gc.Equals, "0014field some data\n0022otherfield more and more data\n")
}

func (*packetV1Suite) TestAppendPacketTooBig(c *gc.C) {
	data, ok := appendPacketV1(nil, "field", make([]byte, 65532))
	c.Assert(ok, gc.Equals, false)
	c.Assert(data, gc.IsNil)
}

var parsePacketV1Tests = []struct {
	data      string
	expect    packetV1
	expectErr string
}{{
	expectErr: "packet too short",
}, {
	data: "0014field some data\n",
	expect: packetV1{
		fieldName: []byte("field"),
		data:      []byte("some data"),
		totalLen:  20,
	},
}, {
	data:      "0015field some data\n",
	expectErr: "packet size too big",
}, {
	data:      "0003a\n",
	expectErr: "packet size too small",
}, {
	data:      "0014fieldwithoutanyspaceordata\n",
	expectErr: "cannot parse field name",
}, {
	data: "fedcsomefield " + strings.Repeat("x", 0xfedc-len("0000somefield \n")) + "\n",
	expect: packetV1{
		fieldName: []byte("somefield"),
		data:      []byte(strings.Repeat("x", 0xfedc-len("0000somefield \n"))),
		totalLen:  0xfedc,
	},
}, {
	data:      "zzzzbadpacketsizenomacaroon",
	expectErr: "cannot parse size",
}}

func (*packetV1Suite) TestParsePacketV1(c *gc.C) {
	for i, test := range parsePacketV1Tests {
		c.Logf("test %d: %q", i, truncate(test.data))
		p, err := parsePacketV1([]byte(test.data))
		if test.expectErr != "" {
			c.Assert(err, gc.ErrorMatches, test.expectErr)
			c.Assert(p, gc.DeepEquals, packetV1{})
			continue
		}
		c.Assert(err, gc.IsNil)
		c.Assert(p, gc.DeepEquals, test.expect)
	}
}

func truncate(d string) string {
	if len(d) > 50 {
		return d[0:50] + "..."
	}
	return d
}

func (*packetV1Suite) TestAsciiHex(c *gc.C) {
	for b := 0; b < 256; b++ {
		n, err := strconv.ParseInt(string(b), 16, 8)
		value, ok := asciiHex(byte(b))
		if err != nil || unicode.IsUpper(rune(b)) {
			c.Assert(ok, gc.Equals, false)
			c.Assert(value, gc.Equals, 0)
		} else {
			c.Assert(ok, gc.Equals, true)
			c.Assert(value, gc.Equals, int(n))
		}
	}
}
