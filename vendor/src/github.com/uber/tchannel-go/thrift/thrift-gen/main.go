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
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/samuel/go-thrift/parser"
)

const tchannelThriftImport = "github.com/uber/tchannel-go/thrift"

var (
	generateThrift = flag.Bool("generateThrift", false, "Whether to generate all Thrift go code")
	inputFile      = flag.String("inputFile", "", "The .thrift file to generate a client for")
	outputDir      = flag.String("outputDir", "gen-go", "The output directory to generate go code to.")
	skipTChannel   = flag.Bool("skipTChannel", false, "Whether to skip the TChannel template")
	templateFiles  = NewStringSliceFlag("template", "Template file to compile code from")

	nlSpaceNL = regexp.MustCompile(`\n[ \t]+\n`)
)

// TemplateData is the data passed to the template that generates code.
type TemplateData struct {
	Package  string
	AST      *parser.Thrift
	Services []*Service
	Includes map[string]*Include
	Imports  imports

	// global should not be directly exported to the template, but functions on
	// global can be exposed to templates.
	global *State
}

type imports struct {
	Thrift   string
	TChannel string
}

func main() {
	flag.Parse()
	if *inputFile == "" {
		log.Fatalf("Please specify an inputFile")
	}

	opts := processOptions{
		InputFile:      *inputFile,
		GenerateThrift: *generateThrift,
		OutputDir:      *outputDir,
		SkipTChannel:   *skipTChannel,
		TemplateFiles:  *templateFiles,
	}
	if err := processFile(opts); err != nil {
		log.Fatal(err)
	}
}

type processOptions struct {
	InputFile      string
	GenerateThrift bool
	OutputDir      string
	SkipTChannel   bool
	TemplateFiles  []string
}

func processFile(opts processOptions) error {
	if err := os.MkdirAll(opts.OutputDir, 0770); err != nil {
		return fmt.Errorf("failed to create output directory %q: %v", opts.OutputDir, err)
	}

	if opts.GenerateThrift {
		if err := runThrift(opts.InputFile, opts.OutputDir); err != nil {
			return fmt.Errorf("failed to run thrift for file %q: %v", opts.InputFile, err)
		}
	}

	allParsed, err := parseFile(opts.InputFile)
	if err != nil {
		return fmt.Errorf("failed to parse file %q: %v", opts.InputFile, err)
	}

	allTemplates, err := parseTemplates(opts.SkipTChannel, opts.TemplateFiles)
	if err != nil {
		return fmt.Errorf("failed to parse templates: %v", err)
	}

	for filename, v := range allParsed {
		pkg := getNamespace(filename, v.ast)

		for _, template := range allTemplates {
			outputFile := filepath.Join(opts.OutputDir, pkg, template.outputFile(pkg))
			if err := generateCode(outputFile, template, pkg, v); err != nil {
				return err
			}
		}
	}

	return nil
}

type parseState struct {
	ast       *parser.Thrift
	namespace string
	global    *State
	services  []*Service
}

// parseTemplates returns a list of Templates that must be rendered given the template files.
func parseTemplates(skipTChannel bool, templateFiles []string) ([]*Template, error) {
	var templates []*Template

	if !skipTChannel {
		templates = append(templates, &Template{
			name:     "tchan",
			template: template.Must(parseTemplate(tchannelTmpl)),
		})
	}

	for _, f := range templateFiles {
		t, err := parseTemplateFile(f)
		if err != nil {
			return nil, err
		}

		templates = append(templates, t)
	}

	return templates, nil
}

func parseFile(inputFile string) (map[string]parseState, error) {
	parser := &parser.Parser{}
	parsed, _, err := parser.ParseFile(inputFile)
	if err != nil {
		return nil, err
	}

	allParsed := make(map[string]parseState)
	for filename, v := range parsed {
		state := newState(v, allParsed)
		services, err := wrapServices(v, state)
		if err != nil {
			return nil, fmt.Errorf("wrap services failed: %v", err)
		}

		namespace := getNamespace(filename, v)
		allParsed[filename] = parseState{v, namespace, state, services}
	}
	setIncludes(allParsed)
	return allParsed, setExtends(allParsed)
}

func defaultPackageName(fullPath string) string {
	filename := filepath.Base(fullPath)
	file := strings.TrimSuffix(filename, filepath.Ext(filename))
	return strings.ToLower(file)
}

func getNamespace(filename string, v *parser.Thrift) string {
	if ns, ok := v.Namespaces["go"]; ok {
		return ns
	}

	// TODO(prashant): Remove any characters that are not valid in Go package names.
	return defaultPackageName(filename)
}

func generateCode(outputFile string, template *Template, pkg string, state parseState) error {
	if outputFile == "" {
		return fmt.Errorf("must speciy an output file")
	}
	if len(state.services) == 0 {
		return nil
	}

	td := TemplateData{
		Package:  pkg,
		AST:      state.ast,
		Includes: state.global.includes,
		Services: state.services,
		global:   state.global,
		Imports: imports{
			Thrift:   *apacheThriftImport,
			TChannel: tchannelThriftImport,
		},
	}
	return template.execute(outputFile, td)
}

type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	return strings.Join(*s, ", ")
}

func (s *stringSliceFlag) Set(in string) error {
	*s = append(*s, in)
	return nil
}

// NewStringSliceFlag creates a new string slice flag. The default value is always nil.
func NewStringSliceFlag(name string, usage string) *[]string {
	var ss stringSliceFlag
	flag.Var(&ss, name, usage)
	return (*[]string)(&ss)
}
