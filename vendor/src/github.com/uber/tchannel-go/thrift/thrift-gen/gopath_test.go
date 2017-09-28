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
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getFakeFS(t *testing.T) string {
	files := []string{
		"src/pkg1/sub/ringpop.thriftgen",
		"src/pkg2/sub/ringpop.thriftgen",
	}

	tempDir, err := ioutil.TempDir("", "thriftgen")
	require.NoError(t, err, "TempDir failed")

	for _, f := range files {
		require.NoError(t, os.MkdirAll(filepath.Join(tempDir, filepath.Dir(f)), 0770),
			"Failed to create directory structure for %v", f)
		require.NoError(t, ioutil.WriteFile(filepath.Join(tempDir, f), nil, 0660),
			"Failed to create dummy file")
	}
	return tempDir
}

func TestGoPathCandidates(t *testing.T) {
	tests := []struct {
		goPath             string
		filename           string
		expectedCandidates []string
	}{
		{
			goPath:   "onepath",
			filename: "github.com/uber/tchannel-go/tchan.thrift-gen",
			expectedCandidates: []string{
				"github.com/uber/tchannel-go/tchan.thrift-gen",
				"onepath/src/github.com/uber/tchannel-go/tchan.thrift-gen",
			},
		},
		{
			goPath:   "onepath:secondpath",
			filename: "github.com/uber/tchannel-go/tchan.thrift-gen",
			expectedCandidates: []string{
				"github.com/uber/tchannel-go/tchan.thrift-gen",
				"onepath/src/github.com/uber/tchannel-go/tchan.thrift-gen",
				"secondpath/src/github.com/uber/tchannel-go/tchan.thrift-gen",
			},
		},
	}

	for _, tt := range tests {
		os.Setenv("GOPATH", tt.goPath)
		candidates := goPathCandidates(tt.filename)
		if !reflect.DeepEqual(candidates, tt.expectedCandidates) {
			t.Errorf("GOPATH=%s FileCandidatesWithGopath(%s) = %q, want %q",
				tt.goPath, tt.filename, candidates, tt.expectedCandidates)
		}
	}
}

func TestResolveWithGoPath(t *testing.T) {
	goPath1 := getFakeFS(t)
	goPath2 := getFakeFS(t)
	os.Setenv("GOPATH", goPath1+string(filepath.ListSeparator)+goPath2)
	defer os.RemoveAll(goPath1)
	defer os.RemoveAll(goPath2)

	tests := []struct {
		filename string
		want     string
		wantErr  bool
	}{
		{
			filename: "pkg1/sub/ringpop.thriftgen",
			want:     filepath.Join(goPath1, "src/pkg1/sub/ringpop.thriftgen"),
		},
		{
			filename: "pkg2/sub/ringpop.thriftgen",
			want:     filepath.Join(goPath1, "src/pkg2/sub/ringpop.thriftgen"),
		},
		{
			filename: filepath.Join(goPath2, "src/pkg2/sub/ringpop.thriftgen"),
			want:     filepath.Join(goPath2, "src/pkg2/sub/ringpop.thriftgen"),
		},
		{
			filename: "pkg3/sub/ringpop.thriftgen",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		file, err := ResolveWithGoPath(tt.filename)
		gotErr := err != nil
		assert.Equal(t, tt.wantErr, gotErr, "%v expected error: %v got: %v", tt.filename, tt.wantErr, err)
		assert.Equal(t, tt.want, file, "%v expected to resolve to %v, got %v", tt.filename, tt.want, file)
	}
}
