package zap_module

// Modified from fx zaplogger

// Copyright (c) 2021 Uber Technologies, Inc.
//
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

import (
	"strings"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
)

func FXZap() fx.Option {
	return fx.WithLogger(
		func(logger *zap.Logger) fxevent.Logger {
			return &zapLogger{Logger: logger.Named("fx").WithOptions(zap.AddCallerSkip(2))}
		},
	)
}

// zapLogger is an Fx event logger that logs events to Zap.
type zapLogger struct {
	Logger *zap.Logger
}

var _ fxevent.Logger = (*zapLogger)(nil)

// LogEvent logs the given event to the provided Zap logger.
func (l *zapLogger) LogEvent(event fxevent.Event) {
	switch e := event.(type) {
	case *fxevent.OnStartExecuting:
		l.Logger.Info("OnStart hook executing - "+e.CallerName,
			zap.String("callee", e.FunctionName),
			zap.String("caller", e.CallerName),
		)
	case *fxevent.OnStartExecuted:
		if e.Err != nil {
			l.Logger.Info("OnStart hook failed - "+e.CallerName,
				zap.String("callee", e.FunctionName),
				zap.String("caller", e.CallerName),
				zap.Error(e.Err),
			)
		} else {
			l.Logger.Info("OnStart hook executed - "+e.CallerName,
				zap.String("callee", e.FunctionName),
				zap.String("caller", e.CallerName),
				zap.Duration("runtime", e.Runtime),
			)
		}
	case *fxevent.OnStopExecuting:
		l.Logger.Info("OnStop hook executing - "+e.CallerName,
			zap.String("callee", e.FunctionName),
			zap.String("caller", e.CallerName),
		)
	case *fxevent.OnStopExecuted:
		if e.Err != nil {
			l.Logger.Info("OnStop hook failed - "+e.CallerName,
				zap.String("callee", e.FunctionName),
				zap.String("caller", e.CallerName),
				zap.Error(e.Err),
			)
		} else {
			l.Logger.Info("OnStop hook executed - "+e.CallerName,
				zap.String("callee", e.FunctionName),
				zap.String("caller", e.CallerName),
				zap.Duration("runtime", e.Runtime),
			)
		}
	case *fxevent.Supplied:
		l.Logger.Info("supplied - "+e.TypeName, zap.String("type", e.TypeName), zap.Error(e.Err))
	case *fxevent.Provided:
		for _, rtype := range e.OutputTypeNames {
			l.Logger.Info("provided - "+rtype,
				zap.String("constructor", e.ConstructorName),
				zap.String("type", rtype),
			)
		}
		if e.Err != nil {
			l.Logger.Error("error encountered while applying options",
				zap.Error(e.Err))
		}
	case *fxevent.Invoking:
		// Do not log stack as it will make logs hard to read.
		l.Logger.Info("invoking - "+e.FunctionName,
			zap.String("function", e.FunctionName))
	case *fxevent.Invoked:
		if e.Err != nil {
			l.Logger.Error("invoke failed - "+e.FunctionName,
				zap.Error(e.Err),
				zap.String("stack", e.Trace),
				zap.String("function", e.FunctionName))
		}
	case *fxevent.Stopping:
		l.Logger.Info("received signal - "+strings.ToUpper(e.Signal.String()),
			zap.String("signal", strings.ToUpper(e.Signal.String())))
	case *fxevent.Stopped:
		if e.Err != nil {
			l.Logger.Error("stop failed", zap.Error(e.Err))
		}
	case *fxevent.RollingBack:
		l.Logger.Error("start failed, rolling back", zap.Error(e.StartErr))
	case *fxevent.RolledBack:
		if e.Err != nil {
			l.Logger.Error("rollback failed", zap.Error(e.Err))
		}
	case *fxevent.Started:
		if e.Err != nil {
			l.Logger.Error("start failed", zap.Error(e.Err))
		} else {
			l.Logger.Info("started")
		}
	case *fxevent.LoggerInitialized:
		if e.Err != nil {
			l.Logger.Error("custom logger initialization failed", zap.Error(e.Err))
		} else {
			l.Logger.Info("initialized custom fxevent.Logger", zap.String("function", e.ConstructorName))
		}
	}
}
