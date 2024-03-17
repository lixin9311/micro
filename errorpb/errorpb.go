package errorpb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"go.uber.org/zap/zapcore"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"pkg.lucas.icu/micro/gateway_middleware"
	request_id "pkg.lucas.icu/micro/grpc_middleware/requestid"
)

const (
	KeyInvalidArg = "ARG"
	KeyRequestID  = "REQUEST_ID"
	KeyInfoField  = "FORM_FIELD"
	KeyStack      = "STACK"
)

// New creates a new Error just like grpc status
func New(c codes.Code, id ...string) *Error {
	strId := ""
	if len(id) > 0 {
		strId = id[0]
	} else {
		strId = c.String()
	}
	e := &Error{
		Code: int32(c),
		Id:   strId,
	}
	return e
}

func (e *Error) Error() string {
	msg := e.Message
	if msg == "" {
		msg = "unknown error"
	}
	return fmt.Sprintf("%s[%s]: %s", codes.Code(e.Code).String(), e.Id, msg)
}

func (e *Error) WithCode(c codes.Code) *Error {
	e.Code = int32(c)
	return e
}

func (e *Error) WithMessage(msg string) *Error {
	e.Message = msg
	return e
}

func (e *Error) WithID(id string) *Error {
	e.Id = id
	return e
}

// WithMeta will add key - val to the meta
func (e *Error) WithMeta(key, val string) *Error {
	if e == nil {
		return nil
	}
	if e.Metadata == nil {
		e.Metadata = map[string]string{}
	}
	e.Metadata[key] = val
	return e
}

func (e *Error) WithDomain(domain string) *Error {
	e.Domain = domain
	return e
}

// WithContext will add request id to the meta
func (e *Error) WithContext(ctx context.Context) *Error {
	if reqID := request_id.ExtractRequestID(ctx); reqID != "" {
		e.WithMeta(KeyRequestID, reqID)
	}
	return e
}

func (e *Error) GRPCStatus() *status.Status {
	if e == nil {
		return status.New(codes.OK, "OK")
	}
	st := status.New(codes.Code(e.Code), e.Message)
	nst, err := st.WithDetails(
		&errdetails.ErrorInfo{
			Reason:   e.Id,
			Domain:   e.Domain,
			Metadata: e.Metadata,
		},
	)
	if err == nil {
		return nst
	}
	return st
}

// MarshalLogObject implements zap.ObjectMarshaler
func (e *Error) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if e == nil {
		return nil
	}
	enc.AddString("message", e.Message)
	enc.AddString("code", codes.Code(e.Code).String())
	enc.AddString("id", e.Id)
	enc.AddString("domain", e.Domain)
	if len(e.Metadata) != 0 {
		enc.AddObject("metadata", mapMarshaler(e.Metadata))
	}
	return nil
}

type mapMarshaler map[string]string

func (m mapMarshaler) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	for k, v := range m {
		enc.AddString(k, v)
	}
	return nil
}

func fromStatus(st *status.Status) *Error {
	if st == nil {
		return nil
	}
	e := &Error{
		Code:    int32(st.Code()),
		Message: st.Message(),
	}
	for _, d := range st.Details() {
		if ei, ok := d.(*errdetails.ErrorInfo); ok {
			e.Id = ei.Reason
			e.Domain = ei.Domain
			e.Metadata = ei.Metadata
		}
	}
	return e
}

// FromError like status.FromError, it tries to convert grpc error into gdserror.
// If err implements the method `GDSStatus() *Error`, it will be returned directly.
// Additionall, context error, validation error, io.EOF, net.OpErr will be parsed into Error.
// If it is a vanilla grpc error, UNKNOWN_GRPC id and the original error message will be embeded.
// Otherwise, ok is false and it will embeds the err to Error
func FromError(err error) (*Error, bool) {
	if err == nil {
		return nil, false
	}

	if e, ok := err.(*Error); ok {
		return e, true
	}

	st, ok := status.FromError(err)
	if !ok {
		return fromCommonError(err)
	}
	return fromStatus(st), true
}

func MustFromError(err error) *Error {
	gerr, _ := FromError(err)
	return gerr
}

func fromCommonError(err error) (*Error, bool) {
	if err == nil {
		return nil, false
	}
	operr := &net.OpError{}
	if errors.Is(err, context.DeadlineExceeded) {
		return New(codes.DeadlineExceeded).WithMessage(err.Error()), true
	} else if errors.Is(err, context.Canceled) {
		return New(codes.Canceled).WithMessage(err.Error()), true
	} else if errors.Is(err, io.EOF) {
		return New(codes.Internal, "EOF").WithMessage(err.Error()), true
	} else if errors.As(err, &operr) {
		return New(codes.Unavailable).WithMessage(err.Error()), true
	}
	return New(codes.Unknown).WithMessage(err.Error()), false
}

func GrpcGWErrorHandler() runtime.ServeMuxOption {
	return runtime.WithErrorHandler(gateway_middleware.NewHTTPErrorHandler(
		func(err error) (codes.Code, proto.Message) {
			pb := MustFromError(err)
			return codes.Code(pb.Code), pb
		},
	))
}

func GrpcGWRoutingErrorHandler() runtime.ServeMuxOption {
	return runtime.WithRoutingErrorHandler(gateway_middleware.NewRoutingErrorHandler(
		func(httpStatus int) error {
			err := New(codes.Internal).WithMessage("Unexpected routing error")
			switch httpStatus {
			case http.StatusBadRequest:
				err.WithCode(codes.InvalidArgument).WithID(codes.InvalidArgument.String()).WithMessage(http.StatusText(httpStatus))
			case http.StatusMethodNotAllowed:
				err.WithCode(codes.Unimplemented).WithID(codes.Unimplemented.String()).WithMessage(http.StatusText(httpStatus))
			case http.StatusNotFound:
				err.WithCode(codes.NotFound).WithID(codes.NotFound.String()).WithMessage(http.StatusText(httpStatus))
			}
			return err
		},
		func(err error) (codes.Code, proto.Message) {
			pb := MustFromError(err)
			return codes.Code(pb.Code), pb
		},
	))
}

func ErrorParser(err error) (zapcore.ObjectMarshaler, bool) {
	return MustFromError(err), true
}
