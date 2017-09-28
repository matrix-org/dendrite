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

package json

import (
	"fmt"
	"reflect"

	"github.com/uber/tchannel-go"

	"github.com/opentracing/opentracing-go"
	"golang.org/x/net/context"
)

var (
	typeOfError   = reflect.TypeOf((*error)(nil)).Elem()
	typeOfContext = reflect.TypeOf((*Context)(nil)).Elem()
)

// Handlers is the map from method names to handlers.
type Handlers map[string]interface{}

// verifyHandler ensures that the given t is a function with the following signature:
// func(json.Context, *ArgType)(*ResType, error)
func verifyHandler(t reflect.Type) error {
	if t.NumIn() != 2 || t.NumOut() != 2 {
		return fmt.Errorf("handler should be of format func(json.Context, *ArgType) (*ResType, error)")
	}

	isStructPtr := func(t reflect.Type) bool {
		return t.Kind() == reflect.Ptr && t.Elem().Kind() == reflect.Struct
	}
	isMap := func(t reflect.Type) bool {
		return t.Kind() == reflect.Map && t.Key().Kind() == reflect.String
	}
	validateArgRes := func(t reflect.Type, name string) error {
		if !isStructPtr(t) && !isMap(t) {
			return fmt.Errorf("%v should be a pointer to a struct, or a map[string]interface{}", name)
		}
		return nil
	}

	if t.In(0) != typeOfContext {
		return fmt.Errorf("arg0 should be of type json.Context")
	}
	if err := validateArgRes(t.In(1), "second argument"); err != nil {
		return err
	}
	if err := validateArgRes(t.Out(0), "first return value"); err != nil {
		return err
	}
	if !t.Out(1).AssignableTo(typeOfError) {
		return fmt.Errorf("second return value should be an error")
	}

	return nil
}

type handler struct {
	handler  reflect.Value
	argType  reflect.Type
	isArgMap bool
	tracer   func() opentracing.Tracer
}

func toHandler(f interface{}) (*handler, error) {
	hV := reflect.ValueOf(f)
	if err := verifyHandler(hV.Type()); err != nil {
		return nil, err
	}
	argType := hV.Type().In(1)
	return &handler{handler: hV, argType: argType, isArgMap: argType.Kind() == reflect.Map}, nil
}

// Register registers the specified methods specified as a map from method name to the
// JSON handler function. The handler functions should have the following signature:
// func(context.Context, *ArgType)(*ResType, error)
func Register(registrar tchannel.Registrar, funcs Handlers, onError func(context.Context, error)) error {
	handlers := make(map[string]*handler)

	handler := tchannel.HandlerFunc(func(ctx context.Context, call *tchannel.InboundCall) {
		h, ok := handlers[string(call.Method())]
		if !ok {
			onError(ctx, fmt.Errorf("call for unregistered method: %s", call.Method()))
			return
		}

		if err := h.Handle(ctx, call); err != nil {
			onError(ctx, err)
		}
	})

	for m, f := range funcs {
		h, err := toHandler(f)
		if err != nil {
			return fmt.Errorf("%v cannot be used as a handler: %v", m, err)
		}
		h.tracer = func() opentracing.Tracer {
			return tchannel.TracerFromRegistrar(registrar)
		}
		handlers[m] = h
		registrar.Register(handler, m)
	}

	return nil
}

// Handle deserializes the JSON arguments and calls the underlying handler.
func (h *handler) Handle(tctx context.Context, call *tchannel.InboundCall) error {
	var headers map[string]string
	if err := tchannel.NewArgReader(call.Arg2Reader()).ReadJSON(&headers); err != nil {
		return fmt.Errorf("arg2 read failed: %v", err)
	}
	tctx = tchannel.ExtractInboundSpan(tctx, call, headers, h.tracer())
	ctx := WithHeaders(tctx, headers)

	var arg3 reflect.Value
	var callArg reflect.Value
	if h.isArgMap {
		arg3 = reflect.New(h.argType)
		// New returns a pointer, but the method accepts the map directly.
		callArg = arg3.Elem()
	} else {
		arg3 = reflect.New(h.argType.Elem())
		callArg = arg3
	}
	if err := tchannel.NewArgReader(call.Arg3Reader()).ReadJSON(arg3.Interface()); err != nil {
		return fmt.Errorf("arg3 read failed: %v", err)
	}

	args := []reflect.Value{reflect.ValueOf(ctx), callArg}
	results := h.handler.Call(args)

	res := results[0].Interface()
	err := results[1].Interface()
	// If an error was returned, we create an error arg3 to respond with.
	if err != nil {
		// TODO(prashantv): More consistent error handling between json/raw/thrift..
		if serr, ok := err.(tchannel.SystemError); ok {
			return call.Response().SendSystemError(serr)
		}

		call.Response().SetApplicationError()
		// TODO(prashant): Allow client to customize the error in more ways.
		res = struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		}{
			Type:    "error",
			Message: err.(error).Error(),
		}
	}

	if err := tchannel.NewArgWriter(call.Response().Arg2Writer()).WriteJSON(ctx.ResponseHeaders()); err != nil {
		return err
	}

	return tchannel.NewArgWriter(call.Response().Arg3Writer()).WriteJSON(res)
}
