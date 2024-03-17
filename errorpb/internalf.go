package errorpb

import (
	"fmt"
	"runtime"
	"runtime/debug"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
)

// Internalf log the message then wrap it with InternalErr
// Stack dump and line number will be recorded in meta STACK and FILE
func Internalf(format string, args ...interface{}) *Error {
	stack := string(debug.Stack())
	_, file, line, ok := runtime.Caller(1)
	if !ok {
		file = "???"
		line = 0
	}
	short := file
	for i := len(file) - 1; i > 0; i-- {
		if file[i] == '/' {
			short = file[i+1:]
			break
		}
	}
	file = fmt.Sprintf("%s:%d", short, line)
	err := fmt.Errorf(format, args...)
	result := New(codes.Internal).WithMessage(err.Error())
	zap.L().Error(err.Error(), zap.Error(result), zap.String("stack", stack), zap.String("file", file))
	return result
}
