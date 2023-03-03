// Copyright 2023 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal

import (
	"context"
	"runtime/trace"

	"github.com/opentracing/opentracing-go"
)

type Trace struct {
	span   opentracing.Span
	region *trace.Region
}

func StartRegion(inCtx context.Context, name string) (Trace, context.Context) {
	region := trace.StartRegion(inCtx, name)
	span, ctx := opentracing.StartSpanFromContext(inCtx, name)
	return Trace{
		span:   span,
		region: region,
	}, ctx
}

func (t Trace) End() {
	t.span.Finish()
	t.region.End()
}

func (t Trace) SetTag(key string, value any) {
	t.span.SetTag(key, value)
}
