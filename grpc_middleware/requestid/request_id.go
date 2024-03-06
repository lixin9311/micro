package request_id

import (
	"context"
	"time"

	"github.com/segmentio/ksuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/metadata"
)

const (
	RequestIDMetadataKey = "x-request-id"
)

func init() {
	ksuid.SetRand(ksuid.FastRander)
}

func ExtractRequestID(ctx context.Context) string {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		reqIDs, ok := md[RequestIDMetadataKey]
		if ok && len(reqIDs) > 0 {
			return reqIDs[0]
		}
	}
	return ""
}

func InjectRequestID(ctx context.Context, id string) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		md = metadata.MD{}
	}
	md[RequestIDMetadataKey] = []string{id}
	return metadata.NewIncomingContext(ctx, metadata.Join(md, metadata.Pairs(RequestIDMetadataKey, id)))
}

func generateRequestID() string {
	if id, err := ksuid.NewRandom(); err != nil {
		grpclog.Errorf("RequestIDInterceptor: failed to generate random request id, %v", err)
		return time.Now().UTC().Format(time.RFC3339Nano)
	} else {
		return id.String()
	}
}

func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			md = metadata.MD{}
		}
		reqID := ExtractRequestID(ctx)
		if reqID == "" {
			reqID = generateRequestID()
		}
		ctx = metadata.NewIncomingContext(ctx, metadata.Join(md, metadata.Pairs(RequestIDMetadataKey, reqID)))
		return handler(ctx, req)
	}
}

func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		reqID := ExtractRequestID(ctx)
		if reqID == "" {
			reqID = generateRequestID()
		}
		md, _ := metadata.FromOutgoingContext(ctx)
		ctx = metadata.NewOutgoingContext(ctx, metadata.Join(md, metadata.Pairs(RequestIDMetadataKey, reqID)))
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
