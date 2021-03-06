// Copyright (c) The go-grpc-middleware Authors.
// Licensed under the Apache License 2.0.

package grpc_validator

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type option struct {
	all            bool
	errTransformer func(error) error
}

type Option func(o *option)

func defaultErrTransformer(e error) error {
	return status.Error(codes.InvalidArgument, e.Error())
}

func WithAll(all bool) Option {
	return func(o *option) {
		o.all = all
	}
}

func WithErrTransformer(fn func(error) error) Option {
	return func(o *option) {
		o.errTransformer = fn
	}
}

// The validateAller interface at protoc-gen-validate main branch.
// See https://github.com/envoyproxy/protoc-gen-validate/pull/468.
type validateAller interface {
	ValidateAll() error
}

// The validate interface starting with protoc-gen-validate v0.6.0.
// See https://github.com/envoyproxy/protoc-gen-validate/pull/455.
type validator interface {
	Validate(all bool) error
}

// The validate interface prior to protoc-gen-validate v0.6.0.
type validatorLegacy interface {
	Validate() error
}

func validate(req interface{}, all bool, errorTransformer func(error) error) error {
	if all {
		switch v := req.(type) {
		case validateAller:
			if err := v.ValidateAll(); err != nil {
				return status.Error(codes.InvalidArgument, err.Error())
			}
		case validator:
			if err := v.Validate(true); err != nil {
				return status.Error(codes.InvalidArgument, err.Error())
			}
		case validatorLegacy:
			// Fallback to legacy validator
			if err := v.Validate(); err != nil {
				return status.Error(codes.InvalidArgument, err.Error())
			}
		}
		return nil
	}
	switch v := req.(type) {
	case validatorLegacy:
		if err := v.Validate(); err != nil {
			return status.Error(codes.InvalidArgument, err.Error())
		}
	case validator:
		if err := v.Validate(false); err != nil {
			return status.Error(codes.InvalidArgument, err.Error())
		}
	}
	return nil
}

// UnaryServerInterceptor returns a new unary server interceptor that validates incoming messages.
//
// Invalid messages will be rejected with `InvalidArgument` before reaching any userspace handlers.
// If `all` is false, the interceptor returns first validation error. Otherwise the interceptor
// returns ALL validation error as a wrapped multi-error.
// Note that generated codes prior to protoc-gen-validate v0.6.0 do not provide an all-validation
// interface. In this case the interceptor fallbacks to legacy validation and `all` is ignored.
func UnaryServerInterceptor(opts ...Option) grpc.UnaryServerInterceptor {
	opt := &option{}
	for _, fn := range opts {
		fn(opt)
	}
	if opt.errTransformer == nil {
		opt.errTransformer = defaultErrTransformer
	}

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if err := validate(req, opt.all, opt.errTransformer); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// UnaryClientInterceptor returns a new unary client interceptor that validates outgoing messages.
//
// Invalid messages will be rejected with `InvalidArgument` before sending the request to server.
// If `all` is false, the interceptor returns first validation error. Otherwise the interceptor
// returns ALL validation error as a wrapped multi-error.
// Note that generated codes prior to protoc-gen-validate v0.6.0 do not provide an all-validation
// interface. In this case the interceptor fallbacks to legacy validation and `all` is ignored.
func UnaryClientInterceptor(opts ...Option) grpc.UnaryClientInterceptor {
	opt := &option{}
	for _, fn := range opts {
		fn(opt)
	}
	if opt.errTransformer == nil {
		opt.errTransformer = defaultErrTransformer
	}

	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if err := validate(req, opt.all, opt.errTransformer); err != nil {
			return err
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// StreamServerInterceptor returns a new streaming server interceptor that validates incoming messages.
//
// If `all` is false, the interceptor returns first validation error. Otherwise the interceptor
// returns ALL validation error as a wrapped multi-error.
// Note that generated codes prior to protoc-gen-validate v0.6.0 do not provide an all-validation
// interface. In this case the interceptor fallbacks to legacy validation and `all` is ignored.
// The stage at which invalid messages will be rejected with `InvalidArgument` varies based on the
// type of the RPC. For `ServerStream` (1:m) requests, it will happen before reaching any userspace
// handlers. For `ClientStream` (n:1) or `BidiStream` (n:m) RPCs, the messages will be rejected on
// calls to `stream.Recv()`.
func StreamServerInterceptor(opts ...Option) grpc.StreamServerInterceptor {
	opt := &option{}
	for _, fn := range opts {
		fn(opt)
	}
	if opt.errTransformer == nil {
		opt.errTransformer = defaultErrTransformer
	}

	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		wrapper := &recvWrapper{
			all:            opt.all,
			errTransformer: opt.errTransformer,
			ServerStream:   stream,
		}
		return handler(srv, wrapper)
	}
}

type recvWrapper struct {
	all            bool
	errTransformer func(error) error
	grpc.ServerStream
}

func (s *recvWrapper) RecvMsg(m interface{}) error {
	if err := s.ServerStream.RecvMsg(m); err != nil {
		return err
	}
	if err := validate(m, s.all, s.errTransformer); err != nil {
		return err
	}
	return nil
}
