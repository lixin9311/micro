package grpc_zap

import (
	"context"
	"path"
	"time"

	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/lixin9311/zapx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type LoggingDecider func(ctx context.Context, fullMethodName string) bool

func UnaryServerInterceptor(logger *zap.Logger, logReq bool, decider LoggingDecider) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		if !decider(ctx, info.FullMethod) {
			return handler(ctx, req)
		}

		// populate basic fields
		fullMethodString := info.FullMethod
		service := path.Dir(fullMethodString)[1:]
		method := path.Base(fullMethodString)
		f1 := []zapcore.Field{
			zap.String("grpc.service", service),
			zap.String("grpc.method", method),
			zapx.Context(ctx),
			zapx.Metadata(ctx),
		}
		if d, ok := ctx.Deadline(); ok {
			f1 = append(f1, zap.Time("grpc.request.deadline", d))
		}

		// append request
		if logReq {
			if pb, ok := req.(proto.Message); ok {
				f1 = append(f1, zap.Reflect("grpc.request", &jsonpbObjectMarshaler{pb: pb}))
			}
		}

		// wrap logger to context
		callLog := logger.Named(service + "." + method).With(f1...)
		startTime := time.Now()
		newCtx := ctxzap.ToContext(ctx, callLog)

		// call handler
		resp, err = handler(newCtx, req)

		// populate response
		code := status.Code(err)
		level := codeToLevel(code)
		duration := time.Since(startTime)
		status := runtime.HTTPStatusFromCode(code)

		request := zapx.HTTPRequestEntry{
			RequestMethod: "POST",
			RequestURL:    method,
			Status:        status,
			Latency:       duration,
		}
		if peer, ok := peer.FromContext(ctx); ok {
			request.RemoteIP = peer.Addr.String()
		}

		f2 := []zap.Field{
			zap.Error(err),
			zapx.Request(request),
		}

		// append request
		if logReq {
			if pb, ok := resp.(proto.Message); ok {
				f2 = append(f2, zap.Reflect("grpc.response", &jsonpbObjectMarshaler{pb: pb}))
			}
		}

		ctxzap.Extract(newCtx).Check(level, code.String()).Write(f2...)

		return resp, err
	}
}

// codeToLevel is the default implementation of gRPC return codes and interceptor log level for server side.
func codeToLevel(code codes.Code) zapcore.Level {
	switch code {
	case codes.OK:
		return zap.InfoLevel
	case codes.Canceled:
		return zap.InfoLevel
	case codes.Unknown:
		return zap.ErrorLevel
	case codes.InvalidArgument:
		return zap.InfoLevel
	case codes.DeadlineExceeded:
		return zap.WarnLevel
	case codes.NotFound:
		return zap.InfoLevel
	case codes.AlreadyExists:
		return zap.InfoLevel
	case codes.PermissionDenied:
		return zap.WarnLevel
	case codes.Unauthenticated:
		return zap.InfoLevel // unauthenticated requests can happen
	case codes.ResourceExhausted:
		return zap.WarnLevel
	case codes.FailedPrecondition:
		return zap.WarnLevel
	case codes.Aborted:
		return zap.WarnLevel
	case codes.OutOfRange:
		return zap.WarnLevel
	case codes.Unimplemented:
		return zap.ErrorLevel
	case codes.Internal:
		return zap.ErrorLevel
	case codes.Unavailable:
		return zap.WarnLevel
	case codes.DataLoss:
		return zap.ErrorLevel
	default:
		return zap.ErrorLevel
	}
}

func PayloadUnaryServerInterceptor(logger *zap.Logger) grpc.UnaryServerInterceptor {
	return grpc_zap.PayloadUnaryServerInterceptor(logger, func(context.Context, string, interface{}) bool { return true })
}

type jsonpbObjectMarshaler struct {
	pb proto.Message
}

func (j *jsonpbObjectMarshaler) MarshalJSON() ([]byte, error) {
	return protojson.Marshal(j.pb)
}
