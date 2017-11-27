package sjson

import (
	"bytes"
	"encoding/hex"
	gojson "encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/Jeffail/gabs"

	jlexer "github.com/mailru/easyjson/jlexer"
	jwriter "github.com/mailru/easyjson/jwriter"
	fflib "github.com/pquerna/ffjson/fflib/v1"
)

func TestInvalidPaths(t *testing.T) {
	var err error
	_, err = SetRaw(`{"hello":"world"}`, "", `"planet"`)
	if err == nil || err.Error() != "path cannot be empty" {
		t.Fatalf("expecting '%v', got '%v'", "path cannot be empty", err)
	}
	_, err = SetRaw("", "name.last.#", "")
	if err == nil || err.Error() != "array access character not allowed in path" {
		t.Fatalf("expecting '%v', got '%v'", "array access character not allowed in path", err)
	}
	_, err = SetRaw("", "name.last.\\1#", "")
	if err == nil || err.Error() != "array access character not allowed in path" {
		t.Fatalf("expecting '%v', got '%v'", "array access character not allowed in path", err)
	}
	_, err = SetRaw("", "name.las?t", "")
	if err == nil || err.Error() != "wildcard characters not allowed in path" {
		t.Fatalf("expecting '%v', got '%v'", "wildcard characters not allowed in path", err)
	}
	_, err = SetRaw("", "name.la\\s?t", "")
	if err == nil || err.Error() != "wildcard characters not allowed in path" {
		t.Fatalf("expecting '%v', got '%v'", "wildcard characters not allowed in path", err)
	}
	_, err = SetRaw("", "name.las*t", "")
	if err == nil || err.Error() != "wildcard characters not allowed in path" {
		t.Fatalf("expecting '%v', got '%v'", "wildcard characters not allowed in path", err)
	}
	_, err = SetRaw("", "name.las\\a*t", "")
	if err == nil || err.Error() != "wildcard characters not allowed in path" {
		t.Fatalf("expecting '%v', got '%v'", "wildcard characters not allowed in path", err)
	}
}

const (
	setRaw    = 1
	setBool   = 2
	setInt    = 3
	setFloat  = 4
	setString = 5
	setDelete = 6
)

func testRaw(t *testing.T, kind int, expect, json, path string, value interface{}) {
	var json2 string
	var err error
	switch kind {
	default:
		json2, err = Set(json, path, value)
	case setRaw:
		json2, err = SetRaw(json, path, value.(string))
	case setDelete:
		json2, err = Delete(json, path)
	}
	if err != nil {
		t.Fatal(err)
	} else if json2 != expect {
		t.Fatalf("expected '%v', got '%v'", expect, json2)
	}

	var json3 []byte
	switch kind {
	default:
		json3, err = SetBytes([]byte(json), path, value)
	case setRaw:
		json3, err = SetRawBytes([]byte(json), path, []byte(value.(string)))
	case setDelete:
		json3, err = DeleteBytes([]byte(json), path)
	}
	if err != nil {
		t.Fatal(err)
	} else if string(json3) != expect {
		t.Fatalf("expected '%v', got '%v'", expect, string(json3))
	}
}
func TestBasic(t *testing.T) {
	testRaw(t, setRaw, `[{"hiw":"planet","hi":"world"}]`, `[{"hi":"world"}]`, "0.hiw", `"planet"`)
	testRaw(t, setRaw, `[true]`, ``, "0", `true`)
	testRaw(t, setRaw, `[null,true]`, ``, "1", `true`)
	testRaw(t, setRaw, `[1,null,true]`, `[1]`, "2", `true`)
	testRaw(t, setRaw, `[1,true,false]`, `[1,null,false]`, "1", `true`)
	testRaw(t, setRaw,
		`[1,{"hello":"when","this":[0,null,2]},false]`,
		`[1,{"hello":"when","this":[0,1,2]},false]`,
		"1.this.1", `null`)
	testRaw(t, setRaw,
		`{"a":1,"b":{"hello":"when","this":[0,null,2]},"c":false}`,
		`{"a":1,"b":{"hello":"when","this":[0,1,2]},"c":false}`,
		"b.this.1", `null`)
	testRaw(t, setRaw,
		`{"a":1,"b":{"hello":"when","this":[0,null,2,null,4]},"c":false}`,
		`{"a":1,"b":{"hello":"when","this":[0,null,2]},"c":false}`,
		"b.this.4", `4`)
	testRaw(t, setRaw,
		`{"b":{"this":[null,null,null,null,4]}}`,
		``,
		"b.this.4", `4`)
	testRaw(t, setRaw,
		`[null,{"this":[null,null,null,null,4]}]`,
		``,
		"1.this.4", `4`)
	testRaw(t, setRaw,
		`{"1":{"this":[null,null,null,null,4]}}`,
		``,
		":1.this.4", `4`)
	testRaw(t, setRaw,
		`{":1":{"this":[null,null,null,null,4]}}`,
		``,
		"\\:1.this.4", `4`)
	testRaw(t, setRaw,
		`{":\1":{"this":[null,null,null,null,{".HI":4}]}}`,
		``,
		"\\:\\\\1.this.4.\\.HI", `4`)
	testRaw(t, setRaw,
		`{"b":{"this":{"😇":""}}}`,
		``,
		"b.this.😇", `""`)
	testRaw(t, setRaw,
		`[ 1,2  ,3]`,
		`  [ 1,2  ] `,
		"-1", `3`)
	testRaw(t, setInt, `[1234]`, ``, `0`, int64(1234))
	testRaw(t, setFloat, `[1234.5]`, ``, `0`, float64(1234.5))
	testRaw(t, setString, `["1234.5"]`, ``, `0`, "1234.5")
	testRaw(t, setBool, `[true]`, ``, `0`, true)
	testRaw(t, setBool, `[null]`, ``, `0`, nil)
	testRaw(t, setString, `{"arr":[1]}`, ``, `arr.-1`, 1)
}

func TestDelete(t *testing.T) {
	testRaw(t, setDelete, `[456]`, `[123,456]`, `0`, nil)
	testRaw(t, setDelete, `[123,789]`, `[123,456,789]`, `1`, nil)
	testRaw(t, setDelete, `[123,456]`, `[123,456,789]`, `-1`, nil)
	testRaw(t, setDelete, `{"a":[123,456]}`, `{"a":[123,456,789]}`, `a.-1`, nil)
	testRaw(t, setDelete, `{"and":"another"}`, `{"this":"that","and":"another"}`, `this`, nil)
	testRaw(t, setDelete, `{"this":"that"}`, `{"this":"that","and":"another"}`, `and`, nil)
	testRaw(t, setDelete, `{}`, `{"and":"another"}`, `and`, nil)
	testRaw(t, setDelete, `{"1":"2"}`, `{"1":"2"}`, `3`, nil)
}

// TestRandomData is a fuzzing test that throws random data at SetRaw
// function looking for panics.
func TestRandomData(t *testing.T) {
	var lstr string
	defer func() {
		if v := recover(); v != nil {
			println("'" + hex.EncodeToString([]byte(lstr)) + "'")
			println("'" + lstr + "'")
			panic(v)
		}
	}()
	rand.Seed(time.Now().UnixNano())
	b := make([]byte, 200)
	for i := 0; i < 2000000; i++ {
		n, err := rand.Read(b[:rand.Int()%len(b)])
		if err != nil {
			t.Fatal(err)
		}
		lstr = string(b[:n])
		SetRaw(lstr, "zzzz.zzzz.zzzz", "123")
	}
}

var exampleJSON = `
{
    "sha": "d25341478381063d1c76e81b3a52e0592a7c997f",
    "commit": {
      "author": {
        "name": "Tom Tom Anderson",
        "email": "tomtom@anderson.edu",
        "date": "2013-06-22T16:30:59Z"
      },
      "committer": {
        "name": "Tom Tom Anderson",
        "email": "jeffditto@anderson.edu",
        "date": "2013-06-22T16:30:59Z"
      },
      "message": "Merge pull request #162 from stedolan/utf8-fixes\n\nUtf8 fixes. Closes #161",
      "tree": {
        "sha": "6ab697a8dfb5a96e124666bf6d6213822599fb40",
        "url": "https://api.github.com/repos/stedolan/jq/git/trees/6ab697a8dfb5a96e124666bf6d6213822599fb40"
      },
      "url": "https://api.github.com/repos/stedolan/jq/git/commits/d25341478381063d1c76e81b3a52e0592a7c997f",
      "comment_count": 0
    }
}
`
var path = "commit.committer.email"
var value = "tomtom@anderson.com"
var rawValue = `"tomtom@anderson.com"`
var rawValueBytes = []byte(rawValue)
var expect = strings.Replace(exampleJSON, "jeffditto@anderson.edu", "tomtom@anderson.com", 1)
var jsonBytes = []byte(exampleJSON)
var jsonBytes2 = []byte(exampleJSON)
var expectBytes = []byte(expect)
var opts = &Options{Optimistic: true}
var optsInPlace = &Options{Optimistic: true, ReplaceInPlace: true}

func BenchmarkSet(t *testing.B) {
	t.ReportAllocs()
	for i := 0; i < t.N; i++ {
		res, err := Set(exampleJSON, path, value)
		if err != nil {
			t.Fatal(err)
		}
		if res != expect {
			t.Fatal("expected '%v', got '%v'", expect, res)
		}
	}
}

func BenchmarkSetRaw(t *testing.B) {
	t.ReportAllocs()
	for i := 0; i < t.N; i++ {
		res, err := SetRaw(exampleJSON, path, rawValue)
		if err != nil {
			t.Fatal(err)
		}
		if res != expect {
			t.Fatal("expected '%v', got '%v'", expect, res)
		}
	}
}

func BenchmarkSetBytes(t *testing.B) {
	t.ReportAllocs()
	for i := 0; i < t.N; i++ {
		res, err := SetBytes(jsonBytes, path, value)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Compare(res, expectBytes) != 0 {
			t.Fatal("expected '%v', got '%v'", expect, res)
		}
	}
}

func BenchmarkSetRawBytes(t *testing.B) {
	t.ReportAllocs()
	for i := 0; i < t.N; i++ {
		res, err := SetRawBytes(jsonBytes, path, rawValueBytes)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Compare(res, expectBytes) != 0 {
			t.Fatal("expected '%v', got '%v'", expect, res)
		}
	}
}

func BenchmarkSetOptimistic(t *testing.B) {
	t.ReportAllocs()
	for i := 0; i < t.N; i++ {
		res, err := SetOptions(exampleJSON, path, value, opts)
		if err != nil {
			t.Fatal(err)
		}
		if res != expect {
			t.Fatal("expected '%v', got '%v'", expect, res)
		}
	}
}

func BenchmarkSetInPlace(t *testing.B) {
	t.ReportAllocs()
	for i := 0; i < t.N; i++ {
		res, err := SetOptions(exampleJSON, path, value, optsInPlace)
		if err != nil {
			t.Fatal(err)
		}
		if res != expect {
			t.Fatal("expected '%v', got '%v'", expect, res)
		}
	}
}

func BenchmarkSetRawOptimistic(t *testing.B) {
	t.ReportAllocs()
	for i := 0; i < t.N; i++ {
		res, err := SetRawOptions(exampleJSON, path, rawValue, opts)
		if err != nil {
			t.Fatal(err)
		}
		if res != expect {
			t.Fatal("expected '%v', got '%v'", expect, res)
		}
	}
}

func BenchmarkSetRawInPlace(t *testing.B) {
	t.ReportAllocs()
	for i := 0; i < t.N; i++ {
		res, err := SetRawOptions(exampleJSON, path, rawValue, optsInPlace)
		if err != nil {
			t.Fatal(err)
		}
		if res != expect {
			t.Fatal("expected '%v', got '%v'", expect, res)
		}
	}
}

func BenchmarkSetBytesOptimistic(t *testing.B) {
	t.ReportAllocs()
	for i := 0; i < t.N; i++ {
		res, err := SetBytesOptions(jsonBytes, path, value, opts)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Compare(res, expectBytes) != 0 {
			t.Fatal("expected '%v', got '%v'", string(expectBytes), string(res))
		}
	}
}

func BenchmarkSetBytesInPlace(t *testing.B) {
	t.ReportAllocs()
	for i := 0; i < t.N; i++ {
		copy(jsonBytes2, jsonBytes)
		res, err := SetBytesOptions(jsonBytes2, path, value, optsInPlace)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Compare(res, expectBytes) != 0 {
			t.Fatal("expected '%v', got '%v'", string(expectBytes), string(res))
		}
	}
}

func BenchmarkSetRawBytesOptimistic(t *testing.B) {
	t.ReportAllocs()
	for i := 0; i < t.N; i++ {
		res, err := SetRawBytesOptions(jsonBytes, path, rawValueBytes, opts)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Compare(res, expectBytes) != 0 {
			t.Fatal("expected '%v', got '%v'", string(expectBytes), string(res))
		}
	}
}

func BenchmarkSetRawBytesInPlace(t *testing.B) {
	t.ReportAllocs()
	for i := 0; i < t.N; i++ {
		copy(jsonBytes2, jsonBytes)
		res, err := SetRawBytesOptions(jsonBytes2, path, rawValueBytes, optsInPlace)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Compare(res, expectBytes) != 0 {
			t.Fatal("expected '%v', got '%v'", string(expectBytes), string(res))
		}
	}
}

const benchJSON = `
{
  "widget": {
    "debug": "on",
    "window": {
      "title": "Sample Konfabulator Widget",
      "name": "main_window",
      "width": 500,
      "height": 500
    },
    "image": { 
      "src": "Images/Sun.png",
      "hOffset": 250,
      "vOffset": 250,
      "alignment": "center"
    },
    "text": {
      "data": "Click Here",
      "size": 36,
      "style": "bold",
      "vOffset": 100,
      "alignment": "center",
      "onMouseUp": "sun1.opacity = (sun1.opacity / 100) * 90;"
    }
  }
}    
`

type BenchStruct struct {
	Widget struct {
		Debug  string `json:"debug"`
		Window struct {
			Title  string `json:"title"`
			Name   string `json:"name"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"window"`
		Image struct {
			Src       string `json:"src"`
			HOffset   int    `json:"hOffset"`
			VOffset   int    `json:"vOffset"`
			Alignment string `json:"alignment"`
		} `json:"image"`
		Text struct {
			Data      string `json:"data"`
			Size      int    `json:"size"`
			Style     string `json:"style"`
			VOffset   int    `json:"vOffset"`
			Alignment string `json:"alignment"`
			OnMouseUp string `json:"onMouseUp"`
		} `json:"text"`
	} `json:"widget"`
}

var benchPaths = []string{
	"widget.window.name",
	"widget.image.hOffset",
	"widget.text.onMouseUp",
}

func Benchmark_SJSON(t *testing.B) {
	opts := Options{Optimistic: true}
	t.ReportAllocs()
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		for _, path := range benchPaths {
			var err error
			switch path {
			case "widget.window.name":
				_, err = SetOptions(benchJSON, path, "1", &opts)
			case "widget.image.hOffset":
				_, err = SetOptions(benchJSON, path, 1, &opts)
			case "widget.text.onMouseUp":
				_, err = SetOptions(benchJSON, path, "1", &opts)
			}
			if err != nil {
				t.Fatal(err)
			}

		}
	}
	t.N *= len(benchPaths)
}

func Benchmark_SJSON_ReplaceInPlace(t *testing.B) {
	data := []byte(benchJSON)
	opts := Options{
		Optimistic:     true,
		ReplaceInPlace: true,
	}
	v1, v2 := []byte(`"1"`), []byte("1")
	t.ReportAllocs()
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		for _, path := range benchPaths {
			var err error
			switch path {
			case "widget.window.name":
				_, err = SetRawBytesOptions(data, path, v1, &opts)
			case "widget.image.hOffset":
				_, err = SetRawBytesOptions(data, path, v2, &opts)
			case "widget.text.onMouseUp":
				_, err = SetRawBytesOptions(data, path, v1, &opts)
			}
			if err != nil {
				t.Fatal(err)
			}

		}
	}
	t.N *= len(benchPaths)
}

func Benchmark_Encoding_JSON_Map(t *testing.B) {
	data := []byte(benchJSON)
	t.ReportAllocs()
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		for _, path := range benchPaths {
			var m map[string]interface{}
			if err := gojson.Unmarshal(data, &m); err != nil {
				t.Fatal(err)
			}
			switch path {
			case "widget.window.name":
				m["widget"].(map[string]interface{})["window"].(map[string]interface{})["name"] = "1"
			case "widget.image.hOffset":
				m["widget"].(map[string]interface{})["image"].(map[string]interface{})["hOffset"] = 1
			case "widget.text.onMouseUp":
				m["widget"].(map[string]interface{})["text"].(map[string]interface{})["onMouseUp"] = "1"
			}
			_, err := gojson.Marshal(&m)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	t.N *= len(benchPaths)
}

func Benchmark_Encoding_JSON_Struct(t *testing.B) {
	data := []byte(benchJSON)
	t.ReportAllocs()
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		for _, path := range benchPaths {
			var v BenchStruct
			if err := gojson.Unmarshal(data, &v); err != nil {
				t.Fatal(err)
			}
			switch path {
			case "widget.window.name":
				v.Widget.Window.Name = "1"
			case "widget.image.hOffset":
				v.Widget.Image.HOffset = 1
			case "widget.text.onMouseUp":
				v.Widget.Text.OnMouseUp = "1"
			}
			_, err := gojson.Marshal(&v)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	t.N *= len(benchPaths)
}

func Benchmark_Gabs(t *testing.B) {
	data := []byte(benchJSON)
	t.ReportAllocs()
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		for _, path := range benchPaths {
			jsonParsed, err := gabs.ParseJSON(data)
			if err != nil {
				t.Fatal(err)
			}
			switch path {
			case "widget.window.name":
				jsonParsed.SetP("1", path)
			case "widget.image.hOffset":
				jsonParsed.SetP(1, path)
			case "widget.text.onMouseUp":
				jsonParsed.SetP("1", path)
			}
			jsonParsed.String()
		}
	}
	t.N *= len(benchPaths)
}

func Benchmark_FFJSON(t *testing.B) {
	data := []byte(benchJSON)
	t.ReportAllocs()
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		for _, path := range benchPaths {
			var v BenchStruct
			if err := v.UnmarshalFFJSONFromData(data); err != nil {
				t.Fatal(err)
			}
			switch path {
			case "widget.window.name":
				v.Widget.Window.Name = "1"
			case "widget.image.hOffset":
				v.Widget.Image.HOffset = 1
			case "widget.text.onMouseUp":
				v.Widget.Text.OnMouseUp = "1"
			}
			_, err := v.MarshalFFJSONFromData()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	t.N *= len(benchPaths)
}

func Benchmark_EasyJSON(t *testing.B) {
	data := []byte(benchJSON)
	t.ReportAllocs()
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		for _, path := range benchPaths {
			var v BenchStruct
			if err := v.UnmarshalEasyJSONFromData(data); err != nil {
				t.Fatal(err)
			}
			switch path {
			case "widget.window.name":
				v.Widget.Window.Name = "1"
			case "widget.image.hOffset":
				v.Widget.Image.HOffset = 1
			case "widget.text.onMouseUp":
				v.Widget.Text.OnMouseUp = "1"
			}
			_, err := v.MarshalEasyJSONFromData()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	t.N *= len(benchPaths)
}

//////////////////////////////////////////////////////////////
// EVERYTHING BELOW IS AUTOGENERATED

// suppress unused package warning
var (
	_ = gojson.RawMessage{}
	_ = jlexer.Lexer{}
	_ = jwriter.Writer{}
)

func easyjsonDbb23193DecodeGithubComTidwallSjson(in *jlexer.Lexer, out *BenchStruct) {
	if in.IsNull() {
		in.Skip()
		return
	}
	in.Delim('{')
	for !in.IsDelim('}') {
		key := in.UnsafeString()
		in.WantColon()
		if in.IsNull() {
			in.Skip()
			in.WantComma()
			continue
		}
		switch key {
		case "widget":
			easyjsonDbb23193Decode(in, &out.Widget)
		default:
			in.SkipRecursive()
		}
		in.WantComma()
	}
	in.Delim('}')
}
func easyjsonDbb23193EncodeGithubComTidwallSjson(out *jwriter.Writer, in BenchStruct) {
	out.RawByte('{')
	first := true
	_ = first
	if !first {
		out.RawByte(',')
	}
	first = false
	out.RawString("\"widget\":")
	easyjsonDbb23193Encode(out, in.Widget)
	out.RawByte('}')
}

// MarshalJSON supports json.Marshaler interface
func (v BenchStruct) MarshalEasyJSONFromData() ([]byte, error) {
	w := jwriter.Writer{}
	easyjsonDbb23193EncodeGithubComTidwallSjson(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

// MarshalEasyJSON supports easyjson.Marshaler interface
func (v BenchStruct) MarshalEasyJSON(w *jwriter.Writer) {
	easyjsonDbb23193EncodeGithubComTidwallSjson(w, v)
}

// UnmarshalJSON supports json.Unmarshaler interface
func (v *BenchStruct) UnmarshalEasyJSONFromData(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjsonDbb23193DecodeGithubComTidwallSjson(&r, v)
	return r.Error()
}

// UnmarshalEasyJSON supports easyjson.Unmarshaler interface
func (v *BenchStruct) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjsonDbb23193DecodeGithubComTidwallSjson(l, v)
}
func easyjsonDbb23193Decode(in *jlexer.Lexer, out *struct {
	Debug  string "json:\"debug\""
	Window struct {
		Title  string "json:\"title\""
		Name   string "json:\"name\""
		Width  int    "json:\"width\""
		Height int    "json:\"height\""
	} "json:\"window\""
	Image struct {
		Src       string "json:\"src\""
		HOffset   int    "json:\"hOffset\""
		VOffset   int    "json:\"vOffset\""
		Alignment string "json:\"alignment\""
	} "json:\"image\""
	Text struct {
		Data      string "json:\"data\""
		Size      int    "json:\"size\""
		Style     string "json:\"style\""
		VOffset   int    "json:\"vOffset\""
		Alignment string "json:\"alignment\""
		OnMouseUp string "json:\"onMouseUp\""
	} "json:\"text\""
}) {
	if in.IsNull() {
		in.Skip()
		return
	}
	in.Delim('{')
	for !in.IsDelim('}') {
		key := in.UnsafeString()
		in.WantColon()
		if in.IsNull() {
			in.Skip()
			in.WantComma()
			continue
		}
		switch key {
		case "debug":
			out.Debug = string(in.String())
		case "window":
			easyjsonDbb23193Decode1(in, &out.Window)
		case "image":
			easyjsonDbb23193Decode2(in, &out.Image)
		case "text":
			easyjsonDbb23193Decode3(in, &out.Text)
		default:
			in.SkipRecursive()
		}
		in.WantComma()
	}
	in.Delim('}')
}
func easyjsonDbb23193Encode(out *jwriter.Writer, in struct {
	Debug  string "json:\"debug\""
	Window struct {
		Title  string "json:\"title\""
		Name   string "json:\"name\""
		Width  int    "json:\"width\""
		Height int    "json:\"height\""
	} "json:\"window\""
	Image struct {
		Src       string "json:\"src\""
		HOffset   int    "json:\"hOffset\""
		VOffset   int    "json:\"vOffset\""
		Alignment string "json:\"alignment\""
	} "json:\"image\""
	Text struct {
		Data      string "json:\"data\""
		Size      int    "json:\"size\""
		Style     string "json:\"style\""
		VOffset   int    "json:\"vOffset\""
		Alignment string "json:\"alignment\""
		OnMouseUp string "json:\"onMouseUp\""
	} "json:\"text\""
}) {
	out.RawByte('{')
	first := true
	_ = first
	if !first {
		out.RawByte(',')
	}
	first = false
	out.RawString("\"debug\":")
	out.String(string(in.Debug))
	if !first {
		out.RawByte(',')
	}
	first = false
	out.RawString("\"window\":")
	easyjsonDbb23193Encode1(out, in.Window)
	if !first {
		out.RawByte(',')
	}
	first = false
	out.RawString("\"image\":")
	easyjsonDbb23193Encode2(out, in.Image)
	if !first {
		out.RawByte(',')
	}
	first = false
	out.RawString("\"text\":")
	easyjsonDbb23193Encode3(out, in.Text)
	out.RawByte('}')
}
func easyjsonDbb23193Decode3(in *jlexer.Lexer, out *struct {
	Data      string "json:\"data\""
	Size      int    "json:\"size\""
	Style     string "json:\"style\""
	VOffset   int    "json:\"vOffset\""
	Alignment string "json:\"alignment\""
	OnMouseUp string "json:\"onMouseUp\""
}) {
	if in.IsNull() {
		in.Skip()
		return
	}
	in.Delim('{')
	for !in.IsDelim('}') {
		key := in.UnsafeString()
		in.WantColon()
		if in.IsNull() {
			in.Skip()
			in.WantComma()
			continue
		}
		switch key {
		case "data":
			out.Data = string(in.String())
		case "size":
			out.Size = int(in.Int())
		case "style":
			out.Style = string(in.String())
		case "vOffset":
			out.VOffset = int(in.Int())
		case "alignment":
			out.Alignment = string(in.String())
		case "onMouseUp":
			out.OnMouseUp = string(in.String())
		default:
			in.SkipRecursive()
		}
		in.WantComma()
	}
	in.Delim('}')
}
func easyjsonDbb23193Encode3(out *jwriter.Writer, in struct {
	Data      string "json:\"data\""
	Size      int    "json:\"size\""
	Style     string "json:\"style\""
	VOffset   int    "json:\"vOffset\""
	Alignment string "json:\"alignment\""
	OnMouseUp string "json:\"onMouseUp\""
}) {
	out.RawByte('{')
	first := true
	_ = first
	if !first {
		out.RawByte(',')
	}
	first = false
	out.RawString("\"data\":")
	out.String(string(in.Data))
	if !first {
		out.RawByte(',')
	}
	first = false
	out.RawString("\"size\":")
	out.Int(int(in.Size))
	if !first {
		out.RawByte(',')
	}
	first = false
	out.RawString("\"style\":")
	out.String(string(in.Style))
	if !first {
		out.RawByte(',')
	}
	first = false
	out.RawString("\"vOffset\":")
	out.Int(int(in.VOffset))
	if !first {
		out.RawByte(',')
	}
	first = false
	out.RawString("\"alignment\":")
	out.String(string(in.Alignment))
	if !first {
		out.RawByte(',')
	}
	first = false
	out.RawString("\"onMouseUp\":")
	out.String(string(in.OnMouseUp))
	out.RawByte('}')
}
func easyjsonDbb23193Decode2(in *jlexer.Lexer, out *struct {
	Src       string "json:\"src\""
	HOffset   int    "json:\"hOffset\""
	VOffset   int    "json:\"vOffset\""
	Alignment string "json:\"alignment\""
}) {
	if in.IsNull() {
		in.Skip()
		return
	}
	in.Delim('{')
	for !in.IsDelim('}') {
		key := in.UnsafeString()
		in.WantColon()
		if in.IsNull() {
			in.Skip()
			in.WantComma()
			continue
		}
		switch key {
		case "src":
			out.Src = string(in.String())
		case "hOffset":
			out.HOffset = int(in.Int())
		case "vOffset":
			out.VOffset = int(in.Int())
		case "alignment":
			out.Alignment = string(in.String())
		default:
			in.SkipRecursive()
		}
		in.WantComma()
	}
	in.Delim('}')
}
func easyjsonDbb23193Encode2(out *jwriter.Writer, in struct {
	Src       string "json:\"src\""
	HOffset   int    "json:\"hOffset\""
	VOffset   int    "json:\"vOffset\""
	Alignment string "json:\"alignment\""
}) {
	out.RawByte('{')
	first := true
	_ = first
	if !first {
		out.RawByte(',')
	}
	first = false
	out.RawString("\"src\":")
	out.String(string(in.Src))
	if !first {
		out.RawByte(',')
	}
	first = false
	out.RawString("\"hOffset\":")
	out.Int(int(in.HOffset))
	if !first {
		out.RawByte(',')
	}
	first = false
	out.RawString("\"vOffset\":")
	out.Int(int(in.VOffset))
	if !first {
		out.RawByte(',')
	}
	first = false
	out.RawString("\"alignment\":")
	out.String(string(in.Alignment))
	out.RawByte('}')
}
func easyjsonDbb23193Decode1(in *jlexer.Lexer, out *struct {
	Title  string "json:\"title\""
	Name   string "json:\"name\""
	Width  int    "json:\"width\""
	Height int    "json:\"height\""
}) {
	if in.IsNull() {
		in.Skip()
		return
	}
	in.Delim('{')
	for !in.IsDelim('}') {
		key := in.UnsafeString()
		in.WantColon()
		if in.IsNull() {
			in.Skip()
			in.WantComma()
			continue
		}
		switch key {
		case "title":
			out.Title = string(in.String())
		case "name":
			out.Name = string(in.String())
		case "width":
			out.Width = int(in.Int())
		case "height":
			out.Height = int(in.Int())
		default:
			in.SkipRecursive()
		}
		in.WantComma()
	}
	in.Delim('}')
}
func easyjsonDbb23193Encode1(out *jwriter.Writer, in struct {
	Title  string "json:\"title\""
	Name   string "json:\"name\""
	Width  int    "json:\"width\""
	Height int    "json:\"height\""
}) {
	out.RawByte('{')
	first := true
	_ = first
	if !first {
		out.RawByte(',')
	}
	first = false
	out.RawString("\"title\":")
	out.String(string(in.Title))
	if !first {
		out.RawByte(',')
	}
	first = false
	out.RawString("\"name\":")
	out.String(string(in.Name))
	if !first {
		out.RawByte(',')
	}
	first = false
	out.RawString("\"width\":")
	out.Int(int(in.Width))
	if !first {
		out.RawByte(',')
	}
	first = false
	out.RawString("\"height\":")
	out.Int(int(in.Height))
	out.RawByte('}')
}
func (mj *BenchStruct) MarshalFFJSONFromData() ([]byte, error) {
	var buf fflib.Buffer
	if mj == nil {
		buf.WriteString("null")
		return buf.Bytes(), nil
	}
	err := mj.MarshalJSONBufFFJSON(&buf)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
func (mj *BenchStruct) MarshalJSONBufFFJSON(buf fflib.EncodingBuffer) error {
	if mj == nil {
		buf.WriteString("null")
		return nil
	}
	var err error
	var obj []byte
	_ = obj
	_ = err
	/* Inline struct. type=struct { Debug string "json:\"debug\""; Window struct { Title string "json:\"title\""; Name string "json:\"name\""; Width int "json:\"width\""; Height int "json:\"height\"" } "json:\"window\""; Image struct { Src string "json:\"src\""; HOffset int "json:\"hOffset\""; VOffset int "json:\"vOffset\""; Alignment string "json:\"alignment\"" } "json:\"image\""; Text struct { Data string "json:\"data\""; Size int "json:\"size\""; Style string "json:\"style\""; VOffset int "json:\"vOffset\""; Alignment string "json:\"alignment\""; OnMouseUp string "json:\"onMouseUp\"" } "json:\"text\"" } kind=struct */
	buf.WriteString(`{"widget":{ "debug":`)
	fflib.WriteJsonString(buf, string(mj.Widget.Debug))
	/* Inline struct. type=struct { Title string "json:\"title\""; Name string "json:\"name\""; Width int "json:\"width\""; Height int "json:\"height\"" } kind=struct */
	buf.WriteString(`,"window":{ "title":`)
	fflib.WriteJsonString(buf, string(mj.Widget.Window.Title))
	buf.WriteString(`,"name":`)
	fflib.WriteJsonString(buf, string(mj.Widget.Window.Name))
	buf.WriteString(`,"width":`)
	fflib.FormatBits2(buf, uint64(mj.Widget.Window.Width), 10, mj.Widget.Window.Width < 0)
	buf.WriteString(`,"height":`)
	fflib.FormatBits2(buf, uint64(mj.Widget.Window.Height), 10, mj.Widget.Window.Height < 0)
	buf.WriteByte('}')
	/* Inline struct. type=struct { Src string "json:\"src\""; HOffset int "json:\"hOffset\""; VOffset int "json:\"vOffset\""; Alignment string "json:\"alignment\"" } kind=struct */
	buf.WriteString(`,"image":{ "src":`)
	fflib.WriteJsonString(buf, string(mj.Widget.Image.Src))
	buf.WriteString(`,"hOffset":`)
	fflib.FormatBits2(buf, uint64(mj.Widget.Image.HOffset), 10, mj.Widget.Image.HOffset < 0)
	buf.WriteString(`,"vOffset":`)
	fflib.FormatBits2(buf, uint64(mj.Widget.Image.VOffset), 10, mj.Widget.Image.VOffset < 0)
	buf.WriteString(`,"alignment":`)
	fflib.WriteJsonString(buf, string(mj.Widget.Image.Alignment))
	buf.WriteByte('}')
	/* Inline struct. type=struct { Data string "json:\"data\""; Size int "json:\"size\""; Style string "json:\"style\""; VOffset int "json:\"vOffset\""; Alignment string "json:\"alignment\""; OnMouseUp string "json:\"onMouseUp\"" } kind=struct */
	buf.WriteString(`,"text":{ "data":`)
	fflib.WriteJsonString(buf, string(mj.Widget.Text.Data))
	buf.WriteString(`,"size":`)
	fflib.FormatBits2(buf, uint64(mj.Widget.Text.Size), 10, mj.Widget.Text.Size < 0)
	buf.WriteString(`,"style":`)
	fflib.WriteJsonString(buf, string(mj.Widget.Text.Style))
	buf.WriteString(`,"vOffset":`)
	fflib.FormatBits2(buf, uint64(mj.Widget.Text.VOffset), 10, mj.Widget.Text.VOffset < 0)
	buf.WriteString(`,"alignment":`)
	fflib.WriteJsonString(buf, string(mj.Widget.Text.Alignment))
	buf.WriteString(`,"onMouseUp":`)
	fflib.WriteJsonString(buf, string(mj.Widget.Text.OnMouseUp))
	buf.WriteByte('}')
	buf.WriteByte('}')
	buf.WriteByte('}')
	return nil
}

const (
	ffj_t_BenchStructbase = iota
	ffj_t_BenchStructno_such_key

	ffj_t_BenchStruct_Widget
)

var ffj_key_BenchStruct_Widget = []byte("widget")

func (uj *BenchStruct) UnmarshalFFJSONFromData(input []byte) error {
	fs := fflib.NewFFLexer(input)
	return uj.UnmarshalJSONFFLexer(fs, fflib.FFParse_map_start)
}

func (uj *BenchStruct) UnmarshalJSONFFLexer(fs *fflib.FFLexer, state fflib.FFParseState) error {
	var err error = nil
	currentKey := ffj_t_BenchStructbase
	_ = currentKey
	tok := fflib.FFTok_init
	wantedTok := fflib.FFTok_init

mainparse:
	for {
		tok = fs.Scan()
		//	println(fmt.Sprintf("debug: tok: %v  state: %v", tok, state))
		if tok == fflib.FFTok_error {
			goto tokerror
		}

		switch state {

		case fflib.FFParse_map_start:
			if tok != fflib.FFTok_left_bracket {
				wantedTok = fflib.FFTok_left_bracket
				goto wrongtokenerror
			}
			state = fflib.FFParse_want_key
			continue

		case fflib.FFParse_after_value:
			if tok == fflib.FFTok_comma {
				state = fflib.FFParse_want_key
			} else if tok == fflib.FFTok_right_bracket {
				goto done
			} else {
				wantedTok = fflib.FFTok_comma
				goto wrongtokenerror
			}

		case fflib.FFParse_want_key:
			// json {} ended. goto exit. woo.
			if tok == fflib.FFTok_right_bracket {
				goto done
			}
			if tok != fflib.FFTok_string {
				wantedTok = fflib.FFTok_string
				goto wrongtokenerror
			}

			kn := fs.Output.Bytes()
			if len(kn) <= 0 {
				// "" case. hrm.
				currentKey = ffj_t_BenchStructno_such_key
				state = fflib.FFParse_want_colon
				goto mainparse
			} else {
				switch kn[0] {

				case 'w':

					if bytes.Equal(ffj_key_BenchStruct_Widget, kn) {
						currentKey = ffj_t_BenchStruct_Widget
						state = fflib.FFParse_want_colon
						goto mainparse
					}

				}

				if fflib.SimpleLetterEqualFold(ffj_key_BenchStruct_Widget, kn) {
					currentKey = ffj_t_BenchStruct_Widget
					state = fflib.FFParse_want_colon
					goto mainparse
				}

				currentKey = ffj_t_BenchStructno_such_key
				state = fflib.FFParse_want_colon
				goto mainparse
			}

		case fflib.FFParse_want_colon:
			if tok != fflib.FFTok_colon {
				wantedTok = fflib.FFTok_colon
				goto wrongtokenerror
			}
			state = fflib.FFParse_want_value
			continue
		case fflib.FFParse_want_value:

			if tok == fflib.FFTok_left_brace || tok == fflib.FFTok_left_bracket || tok == fflib.FFTok_integer || tok == fflib.FFTok_double || tok == fflib.FFTok_string || tok == fflib.FFTok_bool || tok == fflib.FFTok_null {
				switch currentKey {

				case ffj_t_BenchStruct_Widget:
					goto handle_Widget

				case ffj_t_BenchStructno_such_key:
					err = fs.SkipField(tok)
					if err != nil {
						return fs.WrapErr(err)
					}
					state = fflib.FFParse_after_value
					goto mainparse
				}
			} else {
				goto wantedvalue
			}
		}
	}

handle_Widget:

	/* handler: uj.Widget type=struct { Debug string "json:\"debug\""; Window struct { Title string "json:\"title\""; Name string "json:\"name\""; Width int "json:\"width\""; Height int "json:\"height\"" } "json:\"window\""; Image struct { Src string "json:\"src\""; HOffset int "json:\"hOffset\""; VOffset int "json:\"vOffset\""; Alignment string "json:\"alignment\"" } "json:\"image\""; Text struct { Data string "json:\"data\""; Size int "json:\"size\""; Style string "json:\"style\""; VOffset int "json:\"vOffset\""; Alignment string "json:\"alignment\""; OnMouseUp string "json:\"onMouseUp\"" } "json:\"text\"" } kind=struct quoted=false*/

	{
		/* Falling back. type=struct { Debug string "json:\"debug\""; Window struct { Title string "json:\"title\""; Name string "json:\"name\""; Width int "json:\"width\""; Height int "json:\"height\"" } "json:\"window\""; Image struct { Src string "json:\"src\""; HOffset int "json:\"hOffset\""; VOffset int "json:\"vOffset\""; Alignment string "json:\"alignment\"" } "json:\"image\""; Text struct { Data string "json:\"data\""; Size int "json:\"size\""; Style string "json:\"style\""; VOffset int "json:\"vOffset\""; Alignment string "json:\"alignment\""; OnMouseUp string "json:\"onMouseUp\"" } "json:\"text\"" } kind=struct */
		tbuf, err := fs.CaptureField(tok)
		if err != nil {
			return fs.WrapErr(err)
		}

		err = gojson.Unmarshal(tbuf, &uj.Widget)
		if err != nil {
			return fs.WrapErr(err)
		}
	}

	state = fflib.FFParse_after_value
	goto mainparse

wantedvalue:
	return fs.WrapErr(fmt.Errorf("wanted value token, but got token: %v", tok))
wrongtokenerror:
	return fs.WrapErr(fmt.Errorf("ffjson: wanted token: %v, but got token: %v output=%s", wantedTok, tok, fs.Output.String()))
tokerror:
	if fs.BigError != nil {
		return fs.WrapErr(fs.BigError)
	}
	err = fs.Error.ToError()
	if err != nil {
		return fs.WrapErr(err)
	}
	panic("ffjson-generated: unreachable, please report bug.")
done:
	return nil
}
