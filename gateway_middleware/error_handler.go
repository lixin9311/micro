package gateway_middleware

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"strings"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/protobuf/proto"
)

func NewRoutingErrorHandler(statusToErr func(int) error, f func(err error) (codes.Code, proto.Message)) func(ctx context.Context, mux *runtime.ServeMux, marshaler runtime.Marshaler, w http.ResponseWriter, r *http.Request, httpStatus int) {
	handler := NewHTTPErrorHandler(f)
	return func(ctx context.Context, mux *runtime.ServeMux, marshaler runtime.Marshaler, w http.ResponseWriter, r *http.Request, httpStatus int) {
		err := statusToErr(httpStatus)
		handler(ctx, mux, marshaler, w, r, err)
	}
}

// copied from geteway
func NewHTTPErrorHandler(f func(err error) (codes.Code, proto.Message)) func(ctx context.Context, mux *runtime.ServeMux, marshaler runtime.Marshaler, w http.ResponseWriter, r *http.Request, err error) {
	return func(ctx context.Context, mux *runtime.ServeMux, marshaler runtime.Marshaler, w http.ResponseWriter, r *http.Request, err error) {
		// return Internal when Marshal failed
		const fallback = `{"code": 13, "message": "failed to marshal error message"}`

		var customStatus *runtime.HTTPStatusError
		if errors.As(err, &customStatus) {
			err = customStatus.Err
		}

		code, pb := f(err)

		w.Header().Del("Trailer")
		w.Header().Del("Transfer-Encoding")

		contentType := marshaler.ContentType(pb)
		w.Header().Set("Content-Type", contentType)

		buf, merr := marshaler.Marshal(pb)
		if merr != nil {
			grpclog.Errorf("Failed to marshal error message %q: %v", pb, merr)
			w.WriteHeader(http.StatusInternalServerError)
			if _, err := io.WriteString(w, fallback); err != nil {
				grpclog.Errorf("Failed to write response: %v", err)
			}
			return
		}

		md, ok := runtime.ServerMetadataFromContext(ctx)
		if !ok {
			grpclog.Infof("Failed to extract ServerMetadata from context")
		}

		handleForwardResponseServerMetadata(w, mux, md)

		// RFC 7230 https://tools.ietf.org/html/rfc7230#section-4.1.2
		// Unless the request includes a TE header field indicating "trailers"
		// is acceptable, as described in Section 4.3, a server SHOULD NOT
		// generate trailer fields that it believes are necessary for the user
		// agent to receive.
		doForwardTrailers := requestAcceptsTrailers(r)

		if doForwardTrailers {
			handleForwardResponseTrailerHeader(w, md)
			w.Header().Set("Transfer-Encoding", "chunked")
		}

		st := runtime.HTTPStatusFromCode(code)
		if customStatus != nil {
			st = customStatus.HTTPStatus
		}

		w.WriteHeader(st)
		if _, err := w.Write(buf); err != nil {
			grpclog.Infof("Failed to write response: %v", err)
		}

		if doForwardTrailers {
			handleForwardResponseTrailer(w, md)
		}
	}
}

func handleForwardResponseServerMetadata(w http.ResponseWriter, mux *runtime.ServeMux, md runtime.ServerMetadata) {
	for k, vs := range md.HeaderMD {
		h := fmt.Sprintf("%s%s", runtime.MetadataHeaderPrefix, k)
		for _, v := range vs {
			w.Header().Add(h, v)
		}
	}
}

func handleForwardResponseTrailerHeader(w http.ResponseWriter, md runtime.ServerMetadata) {
	for k := range md.TrailerMD {
		tKey := textproto.CanonicalMIMEHeaderKey(fmt.Sprintf("%s%s", runtime.MetadataTrailerPrefix, k))
		w.Header().Add("Trailer", tKey)
	}
}

func handleForwardResponseTrailer(w http.ResponseWriter, md runtime.ServerMetadata) {
	for k, vs := range md.TrailerMD {
		tKey := fmt.Sprintf("%s%s", runtime.MetadataTrailerPrefix, k)
		for _, v := range vs {
			w.Header().Add(tKey, v)
		}
	}
}

func requestAcceptsTrailers(req *http.Request) bool {
	te := req.Header.Get("TE")
	return strings.Contains(strings.ToLower(te), "trailers")
}
