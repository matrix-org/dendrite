package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/samuel/go-thrift/parser"
)

type byServiceName []*Service

func (l byServiceName) Len() int           { return len(l) }
func (l byServiceName) Less(i, j int) bool { return l[i].Service.Name < l[j].Service.Name }
func (l byServiceName) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }

func wrapServices(v *parser.Thrift, state *State) ([]*Service, error) {
	var services []*Service
	for _, s := range v.Services {
		if err := Validate(s); err != nil {
			return nil, err
		}

		services = append(services, &Service{
			Service: s,
			state:   state,
		})
	}

	// Have a stable ordering for services so code generation is consistent.
	sort.Sort(byServiceName(services))
	return services, nil
}

// Service is a wrapper for parser.Service.
type Service struct {
	*parser.Service
	state *State

	// ExtendsService and ExtendsPrefix are set in `setExtends`.
	ExtendsService *Service
	ExtendsPrefix  string

	// methods is a cache of all methods.
	methods []*Method
	// inheritedMethods is a list of inherited method names.
	inheritedMethods []string
}

// ThriftName returns the thrift identifier for this service.
func (s *Service) ThriftName() string {
	return s.Service.Name
}

// Interface returns the name of the interface representing the service.
func (s *Service) Interface() string {
	return "TChan" + goPublicName(s.Name)
}

// ClientStruct returns the name of the unexported struct that satisfies the interface as a client.
func (s *Service) ClientStruct() string {
	return "tchan" + goPublicName(s.Name) + "Client"
}

// ClientConstructor returns the name of the constructor used to create a client.
func (s *Service) ClientConstructor() string {
	return "NewTChan" + goPublicName(s.Name) + "Client"
}

// InheritedClientConstructor returns the name of the constructor used by the generated code
// for inherited services. This allows the parent service to set the service name that should
// be used.
func (s *Service) InheritedClientConstructor() string {
	return "NewTChan" + goPublicName(s.Name) + "InheritedClient"
}

// ServerStruct returns the name of the unexported struct that satisfies TChanServer.
func (s *Service) ServerStruct() string {
	return "tchan" + goPublicName(s.Name) + "Server"
}

// ServerConstructor returns the name of the constructor used to create the TChanServer interface.
func (s *Service) ServerConstructor() string {
	return "NewTChan" + goPublicName(s.Name) + "Server"
}

// HasExtends returns whether this service extends another service.
func (s *Service) HasExtends() bool {
	return s.ExtendsService != nil
}

// ExtendsServicePrefix returns a package selector (if any) for the extended service.
func (s *Service) ExtendsServicePrefix() string {
	if dotIndex := strings.Index(s.Extends, "."); dotIndex > 0 {
		return s.ExtendsPrefix
	}
	return ""
}

type byMethodName []*Method

func (l byMethodName) Len() int           { return len(l) }
func (l byMethodName) Less(i, j int) bool { return l[i].Method.Name < l[j].Method.Name }
func (l byMethodName) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }

// Methods returns the methods on this service, not including methods from inherited services.
func (s *Service) Methods() []*Method {
	if s.methods != nil {
		return s.methods
	}

	for _, m := range s.Service.Methods {
		s.methods = append(s.methods, &Method{m, s, s.state})
	}
	sort.Sort(byMethodName(s.methods))
	return s.methods
}

// InheritedMethods returns names for inherited methods on this service.
func (s *Service) InheritedMethods() []string {
	if s.inheritedMethods != nil {
		return s.inheritedMethods
	}

	for svc := s.ExtendsService; svc != nil; svc = svc.ExtendsService {
		for m := range svc.Service.Methods {
			s.inheritedMethods = append(s.inheritedMethods, m)
		}
	}
	sort.Strings(s.inheritedMethods)

	return s.inheritedMethods
}

// Method is a wrapper for parser.Method.
type Method struct {
	*parser.Method

	service *Service
	state   *State
}

// ThriftName returns the thrift identifier for this function.
func (m *Method) ThriftName() string {
	return m.Method.Name
}

// Name returns the go method name.
func (m *Method) Name() string {
	return goPublicName(m.Method.Name)
}

// HandleFunc is the go method name for the handle function which decodes the payload.
func (m *Method) HandleFunc() string {
	return "handle" + goPublicName(m.Method.Name)
}

// Arguments returns the argument declarations for this method.
func (m *Method) Arguments() []*Field {
	var args []*Field
	for _, f := range m.Method.Arguments {
		args = append(args, &Field{f, m.state})
	}
	return args
}

// Exceptions returns the exceptions that this method may return.
func (m *Method) Exceptions() []*Field {
	var args []*Field
	for _, f := range m.Method.Exceptions {
		args = append(args, &Field{f, m.state})
	}
	return args
}

// HasReturn returns false if this method is declared as void in the Thrift file.
func (m *Method) HasReturn() bool {
	return m.Method.ReturnType != nil
}

// HasExceptions returns true if this method has
func (m *Method) HasExceptions() bool {
	return len(m.Method.Exceptions) > 0
}

func (m *Method) argResPrefix() string {
	return goPublicName(m.service.Name) + m.Name()
}

// ArgsType returns the Go name for the struct used to encode the method's arguments.
func (m *Method) ArgsType() string {
	return m.argResPrefix() + "Args"
}

// ResultType returns the Go name for the struct used to encode the method's result.
func (m *Method) ResultType() string {
	return m.argResPrefix() + "Result"
}

// ArgList returns the argument list for the function.
func (m *Method) ArgList() string {
	args := []string{"ctx " + contextType()}
	for _, arg := range m.Arguments() {
		args = append(args, arg.Declaration())
	}
	return strings.Join(args, ", ")
}

// CallList creates the call to a function satisfying Interface from an Args struct.
func (m *Method) CallList(reqStruct string) string {
	args := []string{"ctx"}
	for _, arg := range m.Arguments() {
		args = append(args, reqStruct+"."+arg.ArgStructName())
	}
	return strings.Join(args, ", ")
}

// RetType returns the go return type of the method.
func (m *Method) RetType() string {
	if !m.HasReturn() {
		return "error"
	}
	return fmt.Sprintf("(%v, %v)", m.state.goType(m.Method.ReturnType), "error")
}

// WrapResult wraps the result variable before being used in the result struct.
func (m *Method) WrapResult(respVar string) string {
	if !m.HasReturn() {
		panic("cannot wrap a return when there is no return mode")
	}

	if m.state.isResultPointer(m.ReturnType) {
		return respVar
	}
	return "&" + respVar
}

// ReturnWith takes the result name and the error name, and generates the return expression.
func (m *Method) ReturnWith(respName string, errName string) string {
	if !m.HasReturn() {
		return errName
	}
	return fmt.Sprintf("%v, %v", respName, errName)
}

// Field is a wrapper for parser.Field.
type Field struct {
	*parser.Field

	state *State
}

// Declaration returns the declaration for this field.
func (a *Field) Declaration() string {
	return fmt.Sprintf("%s %s", a.Name(), a.ArgType())
}

// Name returns the field name.
func (a *Field) Name() string {
	return goName(a.Field.Name)
}

// ArgType returns the Go type for the given field.
func (a *Field) ArgType() string {
	return a.state.goType(a.Type)
}

// ArgStructName returns the name of this field in the Args struct generated by thrift.
func (a *Field) ArgStructName() string {
	return goPublicFieldName(a.Field.Name)
}
