package gomatrix

import (
	"testing"
)

var useridtests = []struct {
	Input  string
	Output string
}{
	{"Alph@Bet_50up", "_alph=40_bet__50up"},     // The doc example
	{"abcdef", "abcdef"},                        // no-op
	{"i_like_pie_", "i__like__pie__"},           // double underscore escaping
	{"ABCDEF", "_a_b_c_d_e_f"},                  // all-caps
	{"!£", "=21=c2=a3"},                         // punctuation and outside ascii range (U+00A3 => c2 a3)
	{"___", "______"},                           // literal underscores
	{"hello-world.", "hello-world."},            // allowed punctuation
	{"5+5=10", "5=2b5=3d10"},                    // equals sign
	{"東方Project", "=e6=9d=b1=e6=96=b9_project"}, // CJK mixed
	{"	foo bar", "=09foo=20bar"}, // whitespace (tab and space)
}

func TestEncodeUserLocalpart(t *testing.T) {
	for _, u := range useridtests {
		out := EncodeUserLocalpart(u.Input)
		if out != u.Output {
			t.Fatalf("TestEncodeUserLocalpart(%s) => Got: %s Expected: %s", u.Input, out, u.Output)
		}
	}
}

func TestDecodeUserLocalpart(t *testing.T) {
	for _, u := range useridtests {
		in, _ := DecodeUserLocalpart(u.Output)
		if in != u.Input {
			t.Fatalf("TestDecodeUserLocalpart(%s) => Got: %s Expected: %s", u.Output, in, u.Input)
		}
	}
}

var errtests = []struct {
	Input string
}{
	{"foo@bar"},         // invalid character @
	{"foo_5bar"},        // invalid character after _
	{"foo_._-bar"},      // multiple invalid characters after _
	{"foo=2Hbar"},       // invalid hex after =
	{"foo=2hbar"},       // invalid hex after = (lower-case)
	{"foo=======2fbar"}, // multiple invalid hex after =
	{"foo=2"},           // end of string after =
	{"foo_"},            // end of string after _
}

func TestDecodeUserLocalpartErrors(t *testing.T) {
	for _, u := range errtests {
		out, err := DecodeUserLocalpart(u.Input)
		if out != "" {
			t.Fatalf("TestDecodeUserLocalpartErrors(%s) => Got: %s Expected: empty string", u.Input, out)
		}
		if err == nil {
			t.Fatalf("TestDecodeUserLocalpartErrors(%s) => Got: nil error Expected: error", u.Input)
		}
	}
}

var localparttests = []struct {
	Input        string
	ExpectOutput string
}{
	{"@foo:bar", "foo"},
	{"@foo:bar:8448", "foo"},
	{"@foo.bar:baz.quuz", "foo.bar"},
}

func TestExtractUserLocalpart(t *testing.T) {
	for _, u := range localparttests {
		out, err := ExtractUserLocalpart(u.Input)
		if err != nil {
			t.Errorf("TestExtractUserLocalpart(%s) => Error: %s", u.Input, err)
			continue
		}
		if out != u.ExpectOutput {
			t.Errorf("TestExtractUserLocalpart(%s) => Got: %s, Want %s", u.Input, out, u.ExpectOutput)
		}
	}
}
