package cmd_module

import (
	"fmt"
	"io"
	"os"
	"strings"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
)

func Module(verbose bool) fx.Option {
	return fx.Options(
		fx.WithLogger(func() fxevent.Logger {
			return &consoleLogger{
				W:       os.Stderr,
				verbose: verbose,
			}
		}),
	)
}

// consoleLogger is an Fx event logger that attempts to write human-readable
// mesasges to the console.
//
// Use this during development.
type consoleLogger struct {
	W       io.Writer
	verbose bool
}

var _ fxevent.Logger = (*consoleLogger)(nil)

func (l *consoleLogger) logf(msg string, args ...interface{}) {
	fmt.Fprintf(l.W, "[Fx] "+msg+"\n", args...)
}

// LogEvent logs the given event to the provided Zap logger.
func (l *consoleLogger) LogEvent(event fxevent.Event) {
	switch e := event.(type) {
	case *fxevent.OnStartExecuting:
		if l.verbose {
			l.logf("HOOK OnStart\t\t%s executing (caller: %s)", e.FunctionName, e.CallerName)
		}
	case *fxevent.OnStartExecuted:
		if e.Err != nil {
			l.logf("HOOK OnStart\t\t%s called by %s failed in %s: %v", e.FunctionName, e.CallerName, e.Runtime, e.Err)
		} else if l.verbose {
			l.logf("HOOK OnStart\t\t%s called by %s ran successfully in %s", e.FunctionName, e.CallerName, e.Runtime)
		}
	case *fxevent.OnStopExecuting:
		if l.verbose {
			l.logf("HOOK OnStop\t\t%s executing (caller: %s)", e.FunctionName, e.CallerName)
		}
	case *fxevent.OnStopExecuted:
		if e.Err != nil {
			l.logf("HOOK OnStop\t\t%s called by %s failed in %s: %v", e.FunctionName, e.CallerName, e.Runtime, e.Err)
		} else if l.verbose {
			l.logf("HOOK OnStop\t\t%s called by %s ran successfully in %s", e.FunctionName, e.CallerName, e.Runtime)
		}
	case *fxevent.Supplied:
		if e.Err != nil {
			l.logf("ERROR\tFailed to supply %v: %v", e.TypeName, e.Err)
		} else if l.verbose {
			l.logf("SUPPLY\t%v", e.TypeName)
		}
	case *fxevent.Provided:
		if l.verbose {
			for _, rtype := range e.OutputTypeNames {
				l.logf("PROVIDE\t%v <= %v", rtype, e.ConstructorName)
			}
		}
		if e.Err != nil {
			l.logf("Error after options were applied: %v", e.Err)
		}
	case *fxevent.Invoking:
		if l.verbose {
			l.logf("INVOKE\t\t%s", e.FunctionName)
		}
	case *fxevent.Invoked:
		if e.Err != nil {
			l.logf("ERROR\t\tfx.Invoke(%v) called from:\n%+vFailed: %v", e.FunctionName, e.Trace, e.Err)
		}
	case *fxevent.Stopping:
		if l.verbose {
			l.logf("%v", strings.ToUpper(e.Signal.String()))
		}
	case *fxevent.Stopped:
		if e.Err != nil {
			l.logf("ERROR\t\tFailed to stop cleanly: %v", e.Err)
		}
	case *fxevent.RollingBack:
		l.logf("ERROR\t\tStart failed, rolling back: %v", e.StartErr)
	case *fxevent.RolledBack:
		if e.Err != nil {
			l.logf("ERROR\t\tCouldn't roll back cleanly: %v", e.Err)
		}
	case *fxevent.Started:
		if e.Err != nil {
			l.logf("ERROR\t\tFailed to start: %v", e.Err)
		} else if l.verbose {
			l.logf("RUNNING")
		}
	case *fxevent.LoggerInitialized:
		if e.Err != nil {
			l.logf("ERROR\t\tFailed to initialize custom logger: %+v", e.Err)
		} else if l.verbose {
			l.logf("LOGGER\tInitialized custom logger from %v", e.ConstructorName)
		}
	}
}
