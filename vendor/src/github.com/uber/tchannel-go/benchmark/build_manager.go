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

package benchmark

import (
	"io/ioutil"
	"os"
	"os/exec"
	"sync"
)

type buildManager struct {
	sync.RWMutex
	builds map[string]*build
}

type build struct {
	once     sync.Once
	mainFile string

	binaryFile string
	buildErr   error
}

func newBuildManager() *buildManager {
	return &buildManager{
		builds: make(map[string]*build),
	}
}

func (m *buildManager) GoBinary(mainFile string) (string, error) {
	m.Lock()
	bld, ok := m.builds[mainFile]
	if !ok {
		bld = &build{mainFile: mainFile}
		m.builds[mainFile] = bld
	}
	m.Unlock()

	bld.once.Do(bld.Build)
	return bld.binaryFile, bld.buildErr
}

func (b *build) Build() {
	tempFile, err := ioutil.TempFile("", "bench")
	if err != nil {
		panic("Failed to create temp file: " + err.Error())
	}
	tempFile.Close()

	buildCmd := exec.Command("go", "build", "-o", tempFile.Name(), b.mainFile)
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		b.buildErr = err
		return
	}

	b.binaryFile = tempFile.Name()
}
