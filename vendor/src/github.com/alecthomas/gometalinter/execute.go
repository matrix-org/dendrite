package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/shlex"
	"gopkg.in/alecthomas/kingpin.v3-unstable"
)

type Vars map[string]string

func (v Vars) Copy() Vars {
	out := Vars{}
	for k, v := range v {
		out[k] = v
	}
	return out
}

func (v Vars) Replace(s string) string {
	for k, v := range v {
		prefix := regexp.MustCompile(fmt.Sprintf("{%s=([^}]*)}", k))
		if v != "" {
			s = prefix.ReplaceAllString(s, "$1")
		} else {
			s = prefix.ReplaceAllString(s, "")
		}
		s = strings.Replace(s, fmt.Sprintf("{%s}", k), v, -1)
	}
	return s
}

// Severity of linter message.
type Severity string

// Linter message severity levels.
const ( // nolint: deadcode
	Error   Severity = "error"
	Warning Severity = "warning"
)

type Issue struct {
	Linter   string   `json:"linter"`
	Severity Severity `json:"severity"`
	Path     string   `json:"path"`
	Line     int      `json:"line"`
	Col      int      `json:"col"`
	Message  string   `json:"message"`
}

func (i *Issue) String() string {
	buf := new(bytes.Buffer)
	err := formatTemplate.Execute(buf, i)
	kingpin.FatalIfError(err, "Invalid output format")
	return buf.String()
}

type linterState struct {
	*Linter
	id       int
	paths    []string
	issues   chan *Issue
	vars     Vars
	exclude  *regexp.Regexp
	include  *regexp.Regexp
	deadline <-chan time.Time
}

func (l *linterState) Partitions() ([][]string, error) {
	command := l.vars.Replace(l.Command)
	cmdArgs, err := parseCommand(command)
	if err != nil {
		return nil, err
	}
	parts, err := l.Linter.PartitionStrategy(cmdArgs, l.paths)
	if err != nil {
		return nil, err
	}
	return parts, nil
}

func runLinters(linters map[string]*Linter, paths []string, concurrency int, exclude, include *regexp.Regexp) (chan *Issue, chan error) {
	errch := make(chan error, len(linters))
	concurrencych := make(chan bool, concurrency)
	incomingIssues := make(chan *Issue, 1000000)
	processedIssues := filterIssuesViaDirectives(
		newDirectiveParser(),
		maybeSortIssues(maybeAggregateIssues(incomingIssues)))

	vars := Vars{
		"duplthreshold":    fmt.Sprintf("%d", config.DuplThreshold),
		"mincyclo":         fmt.Sprintf("%d", config.Cyclo),
		"maxlinelength":    fmt.Sprintf("%d", config.LineLength),
		"min_confidence":   fmt.Sprintf("%f", config.MinConfidence),
		"min_occurrences":  fmt.Sprintf("%d", config.MinOccurrences),
		"min_const_length": fmt.Sprintf("%d", config.MinConstLength),
		"tests":            "",
	}
	if config.Test {
		vars["tests"] = "-t"
	}

	wg := &sync.WaitGroup{}
	id := 1
	for _, linter := range linters {
		deadline := time.After(config.Deadline.Duration())
		state := &linterState{
			Linter:   linter,
			issues:   incomingIssues,
			paths:    paths,
			vars:     vars,
			exclude:  exclude,
			include:  include,
			deadline: deadline,
		}

		partitions, err := state.Partitions()
		if err != nil {
			errch <- err
			continue
		}
		for _, args := range partitions {
			wg.Add(1)
			// Call the goroutine with a copy of the args array so that the
			// contents of the array are not modified by the next iteration of
			// the above for loop
			go func(id int, args []string) {
				concurrencych <- true
				err := executeLinter(id, state, args)
				if err != nil {
					errch <- err
				}
				<-concurrencych
				wg.Done()
			}(id, args)
			id++
		}
	}

	go func() {
		wg.Wait()
		close(incomingIssues)
		close(errch)
	}()
	return processedIssues, errch
}

func executeLinter(id int, state *linterState, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing linter command")
	}

	start := time.Now()
	dbg := namespacedDebug(fmt.Sprintf("[%s.%d]: ", state.Name, id))
	dbg("executing %s", strings.Join(args, " "))
	buf := bytes.NewBuffer(nil)
	command := args[0]
	cmd := exec.Command(command, args[1:]...) // nolint: gas
	cmd.Stdout = buf
	cmd.Stderr = buf
	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to execute linter %s: %s", command, err)
	}

	done := make(chan bool)
	go func() {
		err = cmd.Wait()
		done <- true
	}()

	// Wait for process to complete or deadline to expire.
	select {
	case <-done:

	case <-state.deadline:
		err = fmt.Errorf("deadline exceeded by linter %s (try increasing --deadline)",
			state.Name)
		kerr := cmd.Process.Kill()
		if kerr != nil {
			warning("failed to kill %s: %s", state.Name, kerr)
		}
		return err
	}

	if err != nil {
		dbg("warning: %s returned %s: %s", command, err, buf.String())
	}

	processOutput(dbg, state, buf.Bytes())
	elapsed := time.Since(start)
	dbg("%s linter took %s", state.Name, elapsed)
	return nil
}

func parseCommand(command string) ([]string, error) {
	args, err := shlex.Split(command)
	if err != nil {
		return nil, err
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("invalid command %q", command)
	}
	exe, err := exec.LookPath(args[0])
	if err != nil {
		return nil, err
	}
	return append([]string{exe}, args[1:]...), nil
}

// nolint: gocyclo
func processOutput(dbg debugFunction, state *linterState, out []byte) {
	re := state.regex
	all := re.FindAllSubmatchIndex(out, -1)
	dbg("%s hits %d: %s", state.Name, len(all), state.Pattern)

	cwd, err := os.Getwd()
	if err != nil {
		warning("failed to get working directory %s", err)
	}

	// Create a local copy of vars so they can be modified by the linter output
	vars := state.vars.Copy()

	for _, indices := range all {
		group := [][]byte{}
		for i := 0; i < len(indices); i += 2 {
			var fragment []byte
			if indices[i] != -1 {
				fragment = out[indices[i]:indices[i+1]]
			}
			group = append(group, fragment)
		}

		issue := &Issue{Line: 1, Linter: state.Linter.Name}
		for i, name := range re.SubexpNames() {
			if group[i] == nil {
				continue
			}
			part := string(group[i])
			if name != "" {
				vars[name] = part
			}
			switch name {
			case "path":
				issue.Path = relativePath(cwd, part)

			case "line":
				n, err := strconv.ParseInt(part, 10, 32)
				kingpin.FatalIfError(err, "line matched invalid integer")
				issue.Line = int(n)

			case "col":
				n, err := strconv.ParseInt(part, 10, 32)
				kingpin.FatalIfError(err, "col matched invalid integer")
				issue.Col = int(n)

			case "message":
				issue.Message = part

			case "":
			}
		}
		// TODO: set messageOveride and severity on the Linter instead of reading
		// them directly from the static config
		if m, ok := config.MessageOverride[state.Name]; ok {
			issue.Message = vars.Replace(m)
		}
		if sev, ok := config.Severity[state.Name]; ok {
			issue.Severity = Severity(sev)
		} else {
			issue.Severity = Warning
		}
		if state.exclude != nil && state.exclude.MatchString(issue.String()) {
			continue
		}
		if state.include != nil && !state.include.MatchString(issue.String()) {
			continue
		}
		state.issues <- issue
	}
}

func relativePath(root, path string) string {
	fallback := path
	root = resolvePath(root)
	path = resolvePath(path)
	var err error
	path, err = filepath.Rel(root, path)
	if err != nil {
		warning("failed to make %s a relative path: %s", fallback, err)
		return fallback
	}
	return path
}

func resolvePath(path string) string {
	var err error
	fallback := path
	if !filepath.IsAbs(path) {
		path, err = filepath.Abs(path)
		if err != nil {
			warning("failed to make %s an absolute path: %s", fallback, err)
			return fallback
		}
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		warning("failed to resolve symlinks in %s: %s", fallback, err)
		return fallback
	}
	return path
}

type sortedIssues struct {
	issues []*Issue
	order  []string
}

func (s *sortedIssues) Len() int      { return len(s.issues) }
func (s *sortedIssues) Swap(i, j int) { s.issues[i], s.issues[j] = s.issues[j], s.issues[i] }

// nolint: gocyclo
func (s *sortedIssues) Less(i, j int) bool {
	l, r := s.issues[i], s.issues[j]
	for _, key := range s.order {
		switch key {
		case "path":
			if l.Path > r.Path {
				return false
			}
		case "line":
			if l.Line > r.Line {
				return false
			}
		case "column":
			if l.Col > r.Col {
				return false
			}
		case "severity":
			if l.Severity > r.Severity {
				return false
			}
		case "message":
			if l.Message > r.Message {
				return false
			}
		case "linter":
			if l.Linter > r.Linter {
				return false
			}
		}
	}
	return true
}

func maybeSortIssues(issues chan *Issue) chan *Issue {
	if reflect.DeepEqual([]string{"none"}, config.Sort) {
		return issues
	}
	out := make(chan *Issue, 1000000)
	sorted := &sortedIssues{
		issues: []*Issue{},
		order:  config.Sort,
	}
	go func() {
		for issue := range issues {
			sorted.issues = append(sorted.issues, issue)
		}
		sort.Sort(sorted)
		for _, issue := range sorted.issues {
			out <- issue
		}
		close(out)
	}()
	return out
}
