// Copyright (c) 2015 Uber Technologies, Inc.

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

// thrift-gen generates code for Thrift services that can be used with the
// uber/tchannel/thrift package. thrift-gen generated code relies on the
// Apache Thrift generated code for serialization/deserialization, and should
// be a part of the generated code's package.
package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os/exec"
	"text/template"
)

// Template represents a single thrift-gen template that will be used to generate code.
type Template struct {
	name     string
	template *template.Template
}

// dummyGoType is a function to be passed to the test/template package as the named
// function 'goType'. This named function is overwritten by an actual implementation
// specific to the thrift file being rendered at the time of rendering.
func dummyGoType() string {
	return ""
}

func parseTemplate(contents string) (*template.Template, error) {
	funcs := map[string]interface{}{
		"contextType":   contextType,
		"goPrivateName": goName,
		"goPublicName":  goPublicName,
		"goType":        dummyGoType,
	}
	return template.New("thrift-gen").Funcs(funcs).Parse(contents)
}

func parseTemplateFile(file string) (*Template, error) {
	file, err := ResolveWithGoPath(file)
	if err != nil {
		return nil, err
	}

	bytes, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %q: %v", file, err)
	}

	t, err := parseTemplate(string(bytes))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template in file %q: %v", file, err)
	}

	return &Template{defaultPackageName(file), t}, nil
}

func contextType() string {
	return "thrift.Context"
}

func cleanGeneratedCode(generated []byte) []byte {
	generated = nlSpaceNL.ReplaceAll(generated, []byte("\n"))
	return generated
}

// withStateFuncs adds functions to the template that are dependent upon state.
func (t *Template) withStateFuncs(td TemplateData) *template.Template {
	return t.template.Funcs(map[string]interface{}{
		"goType": td.global.goType,
	})
}

func (t *Template) execute(outputFile string, td TemplateData) error {
	buf := &bytes.Buffer{}
	if err := t.withStateFuncs(td).Execute(buf, td); err != nil {
		return fmt.Errorf("failed to execute template: %v", err)
	}

	generated := cleanGeneratedCode(buf.Bytes())
	if err := ioutil.WriteFile(outputFile, generated, 0660); err != nil {
		return fmt.Errorf("cannot write output file %q: %v", outputFile, err)
	}

	// Run gofmt on the file (ignore any errors)
	exec.Command("gofmt", "-w", outputFile).Run()
	return nil
}

func (t *Template) outputFile(pkg string) string {
	return fmt.Sprintf("%v-%v.go", t.name, pkg)
}
