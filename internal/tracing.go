// Copyright 2024 New Vector Ltd.
// Copyright 2023 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package internal

import (
	"context"
	"runtime/trace"

	"github.com/opentracing/opentracing-go"
)

type Trace struct {
	span   opentracing.Span
	region *trace.Region
	task   *trace.Task
}

func StartTask(inCtx context.Context, name string) (Trace, context.Context) {
	ctx, task := trace.NewTask(inCtx, name)
	span, ctx := opentracing.StartSpanFromContext(ctx, name)
	return Trace{
		span: span,
		task: task,
	}, ctx
}

func StartRegion(inCtx context.Context, name string) (Trace, context.Context) {
	region := trace.StartRegion(inCtx, name)
	span, ctx := opentracing.StartSpanFromContext(inCtx, name)
	return Trace{
		span:   span,
		region: region,
	}, ctx
}

func (t Trace) EndRegion() {
	t.span.Finish()
	if t.region != nil {
		t.region.End()
	}
}

func (t Trace) EndTask() {
	t.span.Finish()
	if t.task != nil {
		t.task.End()
	}
}

func (t Trace) SetTag(key string, value any) {
	t.span.SetTag(key, value)
}
