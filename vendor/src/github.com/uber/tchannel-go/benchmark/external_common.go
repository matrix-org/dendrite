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
	"bufio"
	"io"
	"os"
	"os/exec"
)

var _bm = newBuildManager()

// externalCmd handles communication with an external benchmark client.
type externalCmd struct {
	cmd        *exec.Cmd
	stdoutOrig io.ReadCloser
	stdout     *bufio.Scanner
	stdin      io.WriteCloser
}

func newExternalCmd(mainFile string, benchArgs []string) (*externalCmd, string) {
	bin, err := _bm.GoBinary(BenchmarkDir + mainFile)
	if err != nil {
		panic("failed to compile " + mainFile + ": " + err.Error())
	}

	cmd := exec.Command(bin, benchArgs...)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		panic("failed to create stdout: " + err.Error())
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		panic("failed to create stdin: " + err.Error())
	}

	if err := cmd.Start(); err != nil {
		panic("failed to start external process: " + err.Error())
	}

	stdoutScanner := bufio.NewScanner(stdout)
	if !stdoutScanner.Scan() {
		panic("failed to check if external process started: " + err.Error())
	}

	out := stdoutScanner.Text()
	return &externalCmd{
		cmd:        cmd,
		stdin:      stdin,
		stdout:     stdoutScanner,
		stdoutOrig: stdout,
	}, out
}

func (c *externalCmd) writeAndRead(cmd string) (string, error) {
	if _, err := io.WriteString(c.stdin, cmd+"\n"); err != nil {
		return "", err
	}

	if c.stdout.Scan() {
		return c.stdout.Text(), nil
	}

	return "", c.stdout.Err()
}

func (c *externalCmd) Close() {
	c.stdin.Close()
	c.stdoutOrig.Close()
	c.cmd.Process.Kill()
}
