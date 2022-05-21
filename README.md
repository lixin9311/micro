# micro

定义了一套用于构建微服务的组件框架。
组件之间的依赖关系，和模块化是基于使用 [uber-go/fx][uber-go/fx] 的。

所有的模块都会有一个 `Module()` 的公开接口，通过组合各种模块，可以快速构建微服务。

如果是运行在Cloud Run上，服务需要从系统环境变量 `PORT` 接受监听端口参数。

需要设定 `SERVICE_TYPE` 为 `grpc` 或者 `http` 来指定对外暴露的服务类型。

## ctx_module 提供 `context.Context`

`ctx_module.Module(ctx context.Context)` 将一个 `context.Context` 加入到模块依赖中，
使它能够贯穿到服务的生命周期中。

建议其他模块使用这个 `context.Context` 来判断服务是否已经停止。

## cfg_module 提供 `*viper.Viper`

`cfg_module.Module(path string)` 将 `*viper.Viper` 加入模块依赖中，并且读入指定的yaml格式的配置文件。

这个模块是很多其他模块的依赖。

建议其他模块如果需要使用配置文件的情况，使用 `*viper.Viper` 来管理配置。

使用 `cfg_module.SetDefaultConfig` 来添加默认配置。请注意给配置定义添加 `mapstructure` 的tag。

## svc_module 提供 `svc_module.Service svc_module.Domain svc_module.ProjectID`

`svc_module.Module(projectID, service, domain string)` 将传入的设定转化成可选的三个关于服务的描述参数。

这个模块是很多其他模块的依赖。

如果需要使用上述参数的情况，在依赖中使用 `svc_module.OptionalConfig`

## cmd_module 提供 fx logger

`cmd_module.Module(verbose bool)` 会提供一个简单的 `fxevent.Logger` 用于记录fx的依赖参数，以及错误。

因为比较简便，所以适合cli工具使用。

## zap_module 提供 `*zap.Logger` 以及 `fxevent.Logger`(可选)

依赖 `cfg_module` 和 `svc_module`。

`zap_module.Module()` 会通过 `*viper.Viper` 读入关于日志的配置，并且初始化一个 `*zap.Logger` 将其设置为 `grpclog` 以及 `zap` 的默认日志。

`zap_module.FXZap()` 会构建一个使用 `*zap.Logger` 的 `fxevent.Logger` 用于记录fx的依赖详情。

## trace_module 提供 opencensus 的 tracer 和 stats

依赖 `cfg_module` 和 `svc_module`。

可以通过 `trace_module.WithOpencensusViews` 添加更多自定义的 view。

## http_module 提供 `*echo.Echo` 作为http服务器

依赖 `cfg_module` 和 `svc_module`。

如果没有在 `/` 和 `/healthz` 注册GET的处理函数，则会默认添加一个回复 http 200 的处理函数在这两个位置，为了健康监测。

如果有使用 `trace_module` 则会自动添加trace。

会默认使用 request_id、request_log、recover、cors、prometheus等中间件。

`http_module.Module(true)` 尽管echo不在依赖中，也会强制启动http服务器。

## grpc_module 提供 `*grpc.Server`

依赖 `cfg_module` 和 `svc_module`。

可以搭配 `grpc_module.GRPCServerOptions` 和 `grpc_module.WithServerOptions` 来提供额外的服务器参数。

如果有使用 `trace_module` 则会自动添加trace。

会默认使用 request_id、request_log、validator、recovery、prometheus、reflection等中间件。

`grpc_module.MustDial` 封装了一下 `grpd.Dial`，并且添加了trace。

如果 `*grpc.Server` 没有被使用的话，则不会启用grpc服务器。

## grpc_gateway 提供 `*runtime.ServerMux`

依赖 `cfg_module` 和 `http_module`。

## example

See [example](https://github.com/lixin9311/micro/tree/master/example) for a more comprehensive example.

```go
package main

import (
 "context"
 "fmt"

 "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
 "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
 "go.uber.org/fx"
 "google.golang.org/grpc"
 "github.com/lixin9311/micro/cfg_module"
 "github.com/lixin9311/micro/cmd_module"
 "github.com/lixin9311/micro/ctx_module"
 example "github.com/lixin9311/micro/example/proto"
 "github.com/lixin9311/micro/grpc_module"
 "github.com/lixin9311/micro/gateway_module"
 "github.com/lixin9311/micro/http_module"
 "github.com/lixin9311/micro/trace_module"
 "github.com/lixin9311/micro/zap_module"
)

func main() {
 ctx, cancel := context.WithCancel(context.Background())
 defer cancel()

 app := fx.New(
  cmd_module.Module(false),
  cfg_module.Module(""),
  ctx_module.Module(ctx),
  zap_module.Module(),
  trace_module.Module(),
  grpc_module.Module(),
  http_module.Module(true),
  gateway_module.Module(),
  fx.Provide(
   NewGRPCService,
  ),
  fx.Invoke(
   RegisterGRPCService,
   RegisterGRPCGateway,
  ),
 )
 app.Run()
}

type server struct {
 example.UnimplementedGreeterServer
}

func (s *server) Hello(ctx context.Context, req *example.HelloReq) (resp *example.HelloResp, err error) {
 ctxzap.Info(ctx, "hello here")
 return &example.HelloResp{
  Message: req.GetMessage(),
 }, nil
}

func NewGRPCService() example.GreeterServer {
 return &server{}
}

func RegisterGRPCService(srv *grpc.Server, svc example.GreeterServer) {
 example.RegisterGreeterServer(srv, svc)
}

func RegisterGRPCGateway(lc fx.Lifecycle, cfg grpc_module.Config, gwmux *runtime.ServeMux) error {
 lc.Append(fx.Hook{
  OnStart: func(ctx context.Context) (err error) {
   return example.RegisterGreeterHandler(context.Background(),
    gwmux,
    grpc_module.MustDial(fmt.Sprintf("127.0.0.1:%d", cfg.ListenPort), grpc.WithInsecure()),
   )
  },
 })
 return nil
}
```

[uber-go/fx]: https://github.com/uber-go/fx
