package macaroon

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type packetV2Suite struct{}

var _ = gc.Suite(&packetV2Suite{})

var parsePacketV2Tests = []struct {
	about        string
	data         string
	expectPacket packetV2
	expectData   string
	expectError  string
}{{
	about: "EOS packet",
	data:  "\x00",
	expectPacket: packetV2{
		fieldType: fieldEOS,
	},
}, {
	about: "simple field",
	data:  "\x02\x03xyz",
	expectPacket: packetV2{
		fieldType: 2,
		data:      []byte("xyz"),
	},
}, {
	about:       "empty buffer",
	data:        "",
	expectError: "varint value extends past end of buffer",
}, {
	about:       "varint out of range",
	data:        "\xff\xff\xff\xff\xff\xff\x7f",
	expectError: "varint value out of range",
}, {
	about:       "varint way out of range",
	data:        "\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\x7f",
	expectError: "varint value out of range",
}, {
	about:       "unterminated varint",
	data:        "\x80",
	expectError: "varint value extends past end of buffer",
}, {
	about:       "field data too long",
	data:        "\x01\x02a",
	expectError: "field data extends past end of buffer",
}, {
	about:       "bad data length varint",
	data:        "\x01\xff",
	expectError: "varint value extends past end of buffer",
}}

func (*packetV2Suite) TestParsePacketV2(c *gc.C) {
	for i, test := range parsePacketV2Tests {
		c.Logf("test %d: %v", i, test.about)
		data, p, err := parsePacketV2([]byte(test.data))
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			c.Assert(data, gc.IsNil)
			c.Assert(p, gc.DeepEquals, packetV2{})
		} else {
			c.Assert(err, gc.IsNil)
			c.Assert(p, jc.DeepEquals, test.expectPacket)
		}
	}
}

var parseSectionV2Tests = []struct {
	about string
	data  string

	expectData    string
	expectPackets []packetV2
	expectError   string
}{{
	about: "no packets",
	data:  "\x00",
}, {
	about: "one packet",
	data:  "\x02\x03xyz\x00",
	expectPackets: []packetV2{{
		fieldType: 2,
		data:      []byte("xyz"),
	}},
}, {
	about: "two packets",
	data:  "\x02\x03xyz\x07\x05abcde\x00",
	expectPackets: []packetV2{{
		fieldType: 2,
		data:      []byte("xyz"),
	}, {
		fieldType: 7,
		data:      []byte("abcde"),
	}},
}, {
	about:       "unterminated section",
	data:        "\x02\x03xyz\x07\x05abcde",
	expectError: "section extends past end of buffer",
}, {
	about:       "out of order fields",
	data:        "\x07\x05abcde\x02\x03xyz\x00",
	expectError: "fields out of order",
}, {
	about:       "bad packet",
	data:        "\x07\x05abcde\xff",
	expectError: "varint value extends past end of buffer",
}}

func (*packetV2Suite) TestParseSectionV2(c *gc.C) {
	for i, test := range parseSectionV2Tests {
		c.Logf("test %d: %v", i, test.about)
		data, ps, err := parseSectionV2([]byte(test.data))
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			c.Assert(data, gc.IsNil)
			c.Assert(ps, gc.IsNil)
		} else {
			c.Assert(err, gc.IsNil)
			c.Assert(ps, jc.DeepEquals, test.expectPackets)
		}
	}
}
