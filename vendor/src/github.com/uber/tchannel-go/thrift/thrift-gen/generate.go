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

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	thriftBinary       = flag.String("thriftBinary", "thrift", "Command to use for the Apache Thrift binary")
	apacheThriftImport = flag.String("thriftImport", "github.com/apache/thrift/lib/go/thrift", "Go package to use for the Thrift import")
	packagePrefix      = flag.String("packagePrefix", "", "The package prefix (will be used similar to how Apache Thrift uses it)")
)

func execCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func execThrift(args ...string) error {
	return execCmd(*thriftBinary, args...)
}

func deleteRemote(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() || !strings.HasSuffix(path, "-remote") {
			return nil
		}

		if err := os.RemoveAll(path); err != nil {
			return err
		}
		// Once the directory is deleted, we can skip the rest of it.
		return filepath.SkipDir
	})
}

func runThrift(inFile string, outDir string) error {
	inFile, err := filepath.Abs(inFile)
	if err != nil {
		return err
	}

	// Delete any existing generated code for this Thrift file.
	genDir := filepath.Join(outDir, defaultPackageName(inFile))
	if err := execCmd("rm", "-rf", genDir); err != nil {
		return fmt.Errorf("failed to delete directory %s: %v", genDir, err)
	}

	// Generate the Apache Thrift generated code.
	goArgs := fmt.Sprintf("go:thrift_import=%s,package_prefix=%s", *apacheThriftImport, *packagePrefix)
	if err := execThrift("-r", "--gen", goArgs, "-out", outDir, inFile); err != nil {
		return fmt.Errorf("thrift compile failed: %v", err)
	}

	// Delete the -remote folders.
	if err := deleteRemote(outDir); err != nil {
		return fmt.Errorf("failed to delete -remote folders: %v", err)
	}

	return nil
}
