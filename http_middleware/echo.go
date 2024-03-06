package http_middleware

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/lixin9311/micro/version"
	"github.com/lixin9311/zapx"
	"github.com/segmentio/ksuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc/metadata"
)

func init() {
	ksuid.SetRand(ksuid.FastRander)
}

func Echox(service string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			switch c.Request().RequestURI {
			case "/", "", "/healthz":
				return c.String(http.StatusOK, service+":"+version.Version())
			default:
				return next(c)
			}
		}
	}
}

func EchoRequestID() echo.MiddlewareFunc {
	return middleware.RequestIDWithConfig(middleware.RequestIDConfig{
		Generator: func() string {
			return ksuid.New().String()
		},
		RequestIDHandler: func(c echo.Context, rid string) {
			ctx := c.Request().Context()
			md, ok := metadata.FromIncomingContext(ctx)
			if !ok {
				md = metadata.MD{}
			}
			ctx = metadata.NewIncomingContext(ctx, metadata.Join(md, metadata.Pairs("x-request-id", rid)))
			c.SetRequest(c.Request().Clone(ctx))
		},
	})
}

func WrapMiddleware(m echo.MiddlewareFunc, opts ...LogOption) echo.MiddlewareFunc {
	o := &options{
		filters:      []RequestFilter{ExcludeURLs(defaultSkippedURLs...)},
		headersToLog: []string{"x-request-id"},
	}
	for _, opt := range opts {
		opt(o)
	}
	o.filters = append(o.filters, ExcludeURLs(o.skippedURLs...))

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			for _, f := range o.filters {
				if !f(c.Request().RequestURI) {
					return next(c)
				}
			}
			return m(next)(c)
		}
	}
}

func recoverFunc(l *zap.Logger, c echo.Context) {
	uri := c.Request().RequestURI
	ctx := c.Request().Context()
	reqBody := []byte{}
	if c.Request().Body != nil { // Read
		reqBody, _ = io.ReadAll(c.Request().Body)
	}
	c.Request().Body = io.NopCloser(bytes.NewBuffer(reqBody)) // Reset
	if len(reqBody) > 1024 {
		reqBody = reqBody[:1024]
	}
	if len(uri) > 200 {
		uri = uri[:200]
	}
	if r := recover(); r != nil {
		err, ok := r.(error)
		if !ok {
			err = fmt.Errorf("%v", r)
		}
		l.Error("[PANIC RECOVER]: "+uri, zap.Error(err), zap.String("stack_trace", string(debug.Stack())),
			zapx.Context(ctx), zapx.Metadata(ctx), zap.String("http.body", string(reqBody)))
		c.Error(err)
	}
}

func EchoRequestLogger(logger *zap.Logger, opts ...LogOption) echo.MiddlewareFunc {
	o := &options{
		filters:      []RequestFilter{ExcludeURLs(defaultSkippedURLs...)},
		headersToLog: []string{"x-request-id"},
	}
	for _, opt := range opts {
		opt(o)
	}
	o.filters = append(o.filters, ExcludeURLs(o.skippedURLs...))

	if o.logBody {
		return func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				defer recoverFunc(logger, c)
				for _, f := range o.filters {
					if !f(c.Request().RequestURI) {
						return next(c)
					}
				}

				// Request
				reqBody := []byte{}
				if c.Request().Body != nil { // Read
					reqBody, _ = io.ReadAll(c.Request().Body)
				}
				c.Request().Body = io.NopCloser(bytes.NewBuffer(reqBody)) // Reset

				// Response
				// resBody := new(bytes.Buffer)
				// mw := io.MultiWriter(c.Response().Writer, resBody)
				// writer := &bodyDumpResponseWriter{Writer: mw, ResponseWriter: c.Response().Writer}
				// c.Response().Writer = writer

				start := time.Now()
				err := next(c)
				if err != nil {
					c.Error(err)
				}

				req := c.Request()
				ctx := req.Context()
				res := c.Response()

				l := logger.Info

				if res.Status >= 500 {
					l = logger.Error
				} else if res.Status >= 400 {
					l = logger.Warn
				}

				id := req.Header.Get(echo.HeaderXRequestID)
				if id == "" {
					id = res.Header().Get(echo.HeaderXRequestID)
				}

				fields := []zapcore.Field{
					zapx.Request(zapx.HTTPRequestEntry{
						Request:      req,
						Status:       res.Status,
						ResponseSize: res.Size,
						Latency:      time.Since(start),
						RemoteIP:     c.RealIP(),
					}),
					zap.String("http.body", sanitized(string(reqBody))),
					zap.String("request_id", id),
					zap.Any("http.header", c.Request().Header),
					zapx.Context(ctx),
				}
				if err != nil {
					fields = append(fields, zap.Error(err))
				}

				l(req.URL.Path, fields...)

				return nil
			}
		}
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			defer recoverFunc(logger, c)
			for _, f := range o.filters {
				if !f(c.Request().RequestURI) {
					return next(c)
				}
			}

			start := time.Now()
			err := next(c)
			if err != nil {
				c.Error(err)
			}

			req := c.Request()
			ctx := req.Context()
			res := c.Response()

			l := logger.Info

			if res.Status >= 500 {
				l = logger.Error
			} else if res.Status >= 400 {
				l = logger.Warn
			}

			id := req.Header.Get(echo.HeaderXRequestID)
			if id == "" {
				id = res.Header().Get(echo.HeaderXRequestID)
			}

			fields := []zapcore.Field{
				zapx.Request(zapx.HTTPRequestEntry{
					Request:      req,
					Status:       res.Status,
					ResponseSize: res.Size,
					Latency:      time.Since(start),
					RemoteIP:     c.RealIP(),
				}),
				// zap.String("body", req.)
				zap.String("request_id", id),
				zapx.Context(ctx),
			}
			if err != nil {
				fields = append(fields, zap.Error(err))
			}

			l(req.URL.Path, fields...)

			return nil
		}
	}
}

// lines contain one of these words are omitted.
var sanitizeWords = []string{
	"password",
	"credit_card",
}

func sanitized(s string) string {
	for _, w := range sanitizeWords {
		if strings.Contains(s, w) {
			return "sanitized"
		}
	}
	return s
}
