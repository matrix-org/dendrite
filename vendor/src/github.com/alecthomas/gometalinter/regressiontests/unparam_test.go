package regressiontests

import (
	"testing"
)

func TestUnparam(t *testing.T) {
	t.Parallel()
	source := `package foo

import (
	"errors"
	"log"
	"net/http"
)

type FooType int

func AllUsed(a, b FooType) FooType { return a + b }

func OneUnused(a, b FooType) FooType { return a }

func doWork() {}

var Sink interface{}

func Parent() {
	oneUnused := func(f FooType) {
		doWork()
	}
	Sink = oneUnused
}

func Handler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("hi"))
}

type FooIface interface {
	foo(w http.ResponseWriter, code FooType) error
}

func FooImpl(w http.ResponseWriter, code FooType) error {
	w.Write([]byte("hi"))
	return nil
}

func (f FooType) AllUsed(a FooType) FooType { return f + a }

func (f FooType) OneUnused(a FooType) FooType { return f }

func DummyImpl(f FooType) {}

func PanicImpl(f FooType) { panic("dummy") }

func NonPanicImpl(w http.ResponseWriter, f FooType) {
	for i := 0; i < 10; i++ {
		w.Write([]byte("foo"))
	}
	panic("default")
}

func endlessLoop(w http.ResponseWriter) {
	for {
		w.Write([]byte("foo"))
	}
}

func NonPanicImpl2(w http.ResponseWriter, f FooType) {
	endlessLoop(w)
	panic("unreachable")
}

func throw(v ...interface{}) {}

func ThrowImpl(f FooType) { throw("dummy") }

func ZeroImpl(f FooType) (int, string, []byte) { return 0, "", nil }

func ErrorsImpl(f FooType) error { return errors.New("unimpl") }

const ConstFoo = FooType(123)

func (f FooType) Error() string { return "foo" }

func CustomErrImpl(f FooType) error { return ConstFoo }

func NonConstImpl(f FooType, s string) error { return f }

func LogImpl(f FooType) { log.Print("not implemented") }

type BarFunc func(a FooType, s string) int

func BarImpl(a FooType, s string) int { return int(a) }

func NoName(FooType) { doWork() }

func UnderscoreName(_ FooType) { doWork() }

type BarStruct struct {
	fn func(a FooType, b byte)
}

func BarField(a FooType, b byte) { doWork() }

type Bar2Struct struct {
	st struct {
		fn func(a FooType, r rune)
	}
}

func Bar2Field(a FooType, r rune) { doWork() }

func FuncAsParam(fn func(FooType) string) { fn(0) }

func PassedAsParam(f FooType) string {
	doWork()
	return "foo"
}

func (f FooType) FuncAsParam2(fn func(FooType) []byte) { fn(0) }

func PassedAsParam2(f FooType) []byte {
	doWork()
	return nil
}

type RecursiveIface interface {
	Foo(RecursiveIface)
}

func AsSliceElem(f FooType) []int {
	doWork()
	return nil
}

var SliceElems = []func(FooType) []int{AsSliceElem} `
	expected := Issues{
		Issue{Linter: "unparam", Severity: "warning", Path: "test.go", Line: 13, Col: 19, Message: "parameter b is unused"},
		Issue{Linter: "unparam", Severity: "warning", Path: "test.go", Line: 20, Col: 20, Message: "parameter f is unused"},
		Issue{Linter: "unparam", Severity: "warning", Path: "test.go", Line: 34, Col: 37, Message: "parameter code is unused"},
		Issue{Linter: "unparam", Severity: "warning", Path: "test.go", Line: 41, Col: 28, Message: "parameter a is unused"},
		Issue{Linter: "unparam", Severity: "warning", Path: "test.go", Line: 47, Col: 42, Message: "parameter f is unused"},
		Issue{Linter: "unparam", Severity: "warning", Path: "test.go", Line: 60, Col: 43, Message: "parameter f is unused"},
		Issue{Linter: "unparam", Severity: "warning", Path: "test.go", Line: 79, Col: 30, Message: "parameter s is unused"},
		Issue{Linter: "unparam", Severity: "warning", Path: "test.go", Line: 85, Col: 25, Message: "parameter s is unused"},
		Issue{Linter: "unparam", Severity: "warning", Path: "test.go", Line: 95, Col: 15, Message: "parameter a is unused"},
		Issue{Linter: "unparam", Severity: "warning", Path: "test.go", Line: 95, Col: 26, Message: "parameter b is unused"},
		Issue{Linter: "unparam", Severity: "warning", Path: "test.go", Line: 103, Col: 16, Message: "parameter a is unused"},
		Issue{Linter: "unparam", Severity: "warning", Path: "test.go", Line: 103, Col: 27, Message: "parameter r is unused"},
		Issue{Linter: "unparam", Severity: "warning", Path: "test.go", Line: 123, Col: 18, Message: "parameter f is unused"},
	}
	ExpectIssues(t, "unparam", source, expected)
}
