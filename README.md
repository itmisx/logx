# logx

<p>
  <a href="https://pkg.go.dev/github.com/itmisx/logx"><img src="https://pkg.go.dev/badge/github.com/itmisx/logx.svg" alt="Go Reference"></a>
  <a href="https://golang.org"><img src="https://img.shields.io/badge/go-%3E%3D1.24-00ADD8.svg" alt="Go Version"></a>
  <a href="./LICENSE"><img src="https://img.shields.io/badge/license-MIT-green.svg" alt="License"></a>
</p>

基于 [uber-go/zap](https://github.com/uber-go/zap) 与 [OpenTelemetry](https://opentelemetry.io/) 的 Go 日志与链路追踪库。
一次调用同时写入**本地日志**、**Loki** 和**链路追踪的 span event**，三者通过 `trace_id` 自动关联。

## 目录

- [特性](#特性)
- [架构](#架构)
- [安装](#安装)
- [快速开始](#快速开始)
- [配置](#配置)
- [日志等级](#日志等级)
- [核心用法](#核心用法)
- [链路传递](#链路传递)
- [优雅退出](#优雅退出)
- [后端对接](#后端对接)
- [API 参考](#api-参考)
- [注意事项](#注意事项)
- [License](#license)

## 特性

- **一次调用，三处落地** —— 本地日志、Loki 推送、span event 由同一个 API 完成
- **日志与链路自动关联** —— 日志自动携带 `trace_id` / `span_id`，可在 Grafana 中从 trace 一键跳转到日志
- **五级日志等级** —— `debug` / `info` / `warn` / `error` / `fatal`，支持运行时动态调整
- **通用 OTLP 导出** —— 可对接 Tempo、Jaeger、OpenTelemetry Collector 及各类兼容后端
- **日志文件切分** —— 基于 [lumberjack](https://github.com/natefinch/lumberjack)，支持按大小切分与 cron 定时切分
- **跨服务链路传递** —— 内置 HTTP 注入提取与 Gin 中间件，同时支持 B3 与 W3C Trace Context 两种格式，也可手动传递 header
- **异常自动上报** —— `Start` 返回的 `defer end()` 在 panic 时记录日志与 span 异常，然后原样抛出交由上层处理

## 架构

```mermaid
flowchart LR
    app["业务代码"] -->|"logx.Info(ctx, msg, fields...)"| logx

    logx -->|zap| local["本地日志<br/>console / file"]
    logx -->|"HTTP push"| loki["Loki"]
    logx -->|"OTLP / HTTP"| otlp["Tempo / Jaeger<br/>OTel Collector"]

    loki --> grafana["Grafana"]
    otlp --> grafana

    grafana -.->|"trace_id 关联"| grafana
```

## 安装

```bash
go get -u github.com/itmisx/logx
```

要求 Go 1.24 及以上。

## 快速开始

```go
package main

import (
    "context"

    "github.com/itmisx/logx"
)

func main() {
    logx.Init(logx.Config{
        Level:  "info",
        Output: "console", // 注意：默认为 none，不输出任何日志

        // 开启链路追踪并导出到本地 Tempo / Jaeger
        EnableTrace:      true,
        TraceSampleRatio: 1,
        OTLPEndpoint:     "localhost:4318",
        OLTPInsecure:     true,
    }, "my-service", logx.String("service.version", "v1.0.0"))

    // 进程退出前导出缓存中的 span
    defer logx.Shutdown(context.Background())

    foo(context.Background())
}

func foo(ctx context.Context) {
    // 启动一个 span，必须配套 defer end()
    ctx, end := logx.Start(ctx, "foo", logx.String("biz", "demo"))
    defer end()

    logx.Info(ctx, "处理开始", logx.Int("user_id", 1001))

    if err := doSomething(ctx); err != nil {
        logx.Error(ctx, "处理失败", logx.Err(err))
        return
    }

    logx.Info(ctx, "处理完成")
}
```

## 配置

### 日志输出

| 字段 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `Level` | `string` | 由 `Debug` 推导 | 日志等级，`debug` / `info` / `warn` / `error` / `fatal` |
| `Output` | `string` | `none` | 输出方式，`none` 不输出、`console` 输出到控制台、`file` 输出到文件 |
| `Debug` | `bool` | `false` | **已废弃**，请使用 `Level`。仅在 `Level` 为空时生效 |

> `Output` 默认为 `none`，即**不输出任何本地日志**。需要本地日志时必须显式设置。

### 日志文件与切分

仅在 `Output` 为 `file` 时生效。

| 字段 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `File` | `string` | `./logs/run.log` | 日志文件路径 |
| `MaxSize` | `int` | `100` | 单个日志文件大小上限，单位 MB，超过后触发切分 |
| `MaxBackups` | `int` | `15` | 保留的切分文件数量上限，超出的被删除 |
| `MaxAge` | `int` | `7` | 切分文件保留天数，超出的被删除 |
| `Compress` | `bool` | `false` | 是否对切分后的文件做 gzip 压缩 |
| `Rotate` | `string` | 空 | 定时切分的 cron 表达式，精确到秒，如 `0 0 0 * * *` 表示每天零点 |

### Loki 推送

| 字段 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `LokiServer` | `string` | 空 | Loki 的 push 地址，如 `http://localhost:3100/loki/api/v1/push`。为空则不推送 |
| `LokiUsername` | `string` | 空 | Basic Auth 用户名 |
| `LokiPassword` | `string` | 空 | Basic Auth 密码 |

服务名与 `Init` 传入的字符串型应用属性会作为 Loki label 上报，label 名需匹配 `^[0-9A-Za-z_]+$`，含 `.` 的属性（如 `service.version`）会被过滤。

### 链路追踪

| 字段 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `EnableTrace` | `bool` | `false` | 追踪总开关。关闭时 `Start` 返回 noop span |
| `TracerProviderType` | `string` | `oltp` | 导出方式，`oltp` 走 OTLP/HTTP、`file` 写入本地 `trace.txt` |
| `TraceSampleRatio` | `float64` | `1` | 根 span 的采样率，取值 `0.0`–`1.0`。**不在 `(0,1]` 范围内（含未设置）时按全量采样处理**，要关闭追踪请用 `EnableTrace` |
| `OTLPEndpoint` | `string` | `localhost:4318` | OTLP 地址，格式为 `host:port`，**不含 scheme 和 path** |
| `OTLPEndpointURLPath` | `string` | `/v1/traces` | OTLP 的 URL path |
| `OLTPInsecure` | `bool` | `false` | 为 `true` 时使用 HTTP，默认使用 HTTPS |
| `OTLPToken` | `string` | 空 | Basic Auth token，非空时作为 `Authorization: Basic <token>` 请求头发送 |

## 日志等级

等级由低到高为 `debug` < `info` < `warn` < `error` < `fatal`，低于配置等级的日志会被丢弃，且**同时作用于本地日志、Loki 推送和 span event**。

```go
logx.Init(logx.Config{Level: "info"}, "my-service")

// 运行时动态调整，并发安全，无需重启
logx.SetLevel("debug")
logx.GetLevel() // "debug"
```

- `fatal` 为最高等级，不受过滤，任何配置下都会被记录
- 等级值大小写不敏感，`warning` 为 `warn` 的别名
- 无法识别的等级会打印一条告警并回退为 `info`
- `Level` 为空时由已废弃的 `Debug` 字段推导：`true` 等价于 `debug`，`false` 等价于 `error`

## 核心用法

### span 的生命周期

```go
func handle(ctx context.Context) {
    // Start 返回携带 span 的新 context，以及结束该 span 的函数
    // 第三个参数起为 span 的附加属性
    ctx, end := logx.Start(ctx, "handle", logx.String("key", "value"))
    // end 结束 span，并在 panic 时记录异常，必须 defer，否则会造成 span 泄露
    defer end()

    // 后续调用传入该 ctx，日志才能挂到这个 span 上
    logx.Info(ctx, "message", logx.Int("count", 3))
}
```

将 `Start` 返回的 ctx 传给下一层，即可形成父子 span：

```go
parentCtx, endParent := logx.Start(context.Background(), "parent")
defer endParent()

childCtx, endChild := logx.Start(parentCtx, "child") // child 的父 span 为 parent
defer endChild()

logx.Info(childCtx, "日志记录到最内层的 child span 上")
```

### 记录日志

```go
logx.Debug(ctx, "调试信息", logx.Any("payload", obj))
logx.Info(ctx, "普通信息", logx.String("user", "alice"))
logx.Warn(ctx, "警告信息", logx.Int("retry", 2))
logx.Error(ctx, "错误信息", logx.Err(err))  // 在 span 上记为 exception
logx.Fatal(ctx, "致命错误")                  // 记录后以退出码 1 结束进程
```

映射关系：

| 方法 | 本地日志 | Loki | 链路追踪 |
| --- | --- | --- | --- |
| `Debug` / `Info` / `Warn` | zap 对应等级 | 对应 level label | `span.AddEvent` |
| `Error` | `error` | `error` | `span.RecordError`（OTel exception 语义） |
| `Fatal` | `fatal` | `fatal` | `span.RecordError` + 强制导出 + 退出进程 |
| panic（由 `end()` 捕获） | `error` + 堆栈 | `error` | `span.RecordError` + 状态置为 `Error` |

`logx.Error` 只在 span 上记一条 exception 事件，**不会**把整个 span 标记为失败——业务错误常常是已处理的。只有 panic 会把 span 状态置为 `Error`，在 trace 视图中标红。

panic 的记录不受 `Level` 过滤，且 `end()` 记录完会**原样重新抛出**，不会吞掉 panic——因此 `gin.Recovery()` 等上层中间件仍能正常接管并返回 500。嵌套多层 `Start` 时，每一层都会记录一次并标红自己的 span。

### 动态补充 span 属性

```go
ctx, end := logx.Start(ctx, "query")
defer end()

// 在 span 执行过程中追加属性
logx.SetSpanAttr(ctx, logx.Int("rows", len(rows)))
```

### 获取 trace 标识

```go
logx.TraceID(ctx) // 当前 trace 的 ID
logx.SpanID(ctx)  // 当前 span 的 ID
```

本地日志与 Loki 推送会自动携带 `trace_id` 与 `span_id` 字段，无需手动添加。

这两个函数以及 `SetSpanAttr`、各日志方法都会**优先取 `Start` 创建的 span，取不到时回退到 context 中的 OpenTelemetry span**。因此 `logx.GinMiddleware` 以及 `otelhttp`、`otelgrpc` 等第三方 instrumentation 创建的 span 都能直接使用，不必为了拿 traceID 额外包一层 `Start`。

> `end` 只结束它自己那次 `Start` 创建的 span，不会误结束中间件或第三方库持有的 span。

## 链路传递

跨进程传递的本质是把 `trace_id` 与 `span_id` 通过载体带到下游。

### HTTP 客户端注入

```go
func post(ctx context.Context) error {
    req, err := http.NewRequest("POST", url, body)
    if err != nil {
        return err
    }
    // 将当前 span 信息注入请求头
    if err := logx.HttpInject(ctx, req); err != nil {
        return err
    }
    _, err = http.DefaultClient.Do(req)
    return err
}
```

### Gin 服务端提取

```go
router := gin.New()
// 从请求头提取上游 span 信息并创建 server span
    // 服务名由 Init 提供，已写入 span 的 resource，这里无需再传
router.Use(logx.GinMiddleware())

func handler(c *gin.Context) {
    ctx := c.Request.Context()

    // 中间件创建的 span 可直接使用，无需再调 Start
    // 日志会带上 trace_id，并作为 event 挂到该 span 上
    logx.Info(ctx, "received")

    // traceID 的三种取法，结果一致
    logx.TraceID(ctx)             // 从 context
    c.GetString(logx.TraceIDKey)  // 从 gin 上下文
    // 调用方则可从响应头 trace-id 获取
}
```

中间件会把 traceID 写入 gin 上下文与响应头 `trace-id`（常量 `logx.TraceIDKey`），追踪未开启时不写出。

需要更细的分段时再调 `Start`，它会成为中间件 span 的子 span：

```go
func handler(c *gin.Context) {
    ctx, end := logx.Start(c.Request.Context(), "queryDB")
    defer end()

    logx.Info(ctx, "查询开始")
}
```

> `logx.GinMiddleware` 使用与 `HttpInject` / `TraceHeaders` 完全一致的 propagator，无需额外配置。
> 需要自定义 propagator 或 tracer provider 时，可直接使用
> `github.com/itmisx/logx/propagation/extract` 下的 `GinMiddleware(service, extract.WithPropagators(...))`，
> 其 `service` 参数对应 semconv 的 `http.server_name`，语义是虚拟主机名而非服务名。

### 手动传递 header

适用于无法直接使用 `HttpInject` 的场景，例如消息队列、gRPC metadata、自行封装的 HTTP 客户端。

```go
// 上游：导出当前 span 的追踪 header
func produce(ctx context.Context, msg *Message) {
    for k, v := range logx.TraceHeaders(ctx) {
        msg.Header.Set(k, v)
    }
}

// 下游：从 header 还原 context，再 Start 即为上游 span 的子 span
func consume(msg *Message) {
    ctx := logx.ContextFromHeaders(context.Background(), msg.Headers())
    ctx, end := logx.Start(ctx, "consume")
    defer end()

    logx.Info(ctx, "开始消费")
}
```

- `TraceHeaders` 在**未开启追踪时返回 `nil`** —— 此时 span 为 noop，trace/span ID 全为 0，不会向下游发送无效 header
- 返回的 key 为 HTTP 规范化形式（`B3`、`Traceparent`），用于 gRPC metadata 等要求小写 key 的场景时需自行转换
- `ContextFromHeaders` 的 key **大小写不敏感**，B3 单 header、B3 多 header、W3C `traceparent` 三种格式都能识别
- header 中没有有效追踪信息时原样返回传入的 ctx，后续 `Start` 会开启一条新的 trace

服务端也可直接从 `*http.Request` 提取，与 `HttpInject` 对应：

```go
func handler(w http.ResponseWriter, r *http.Request) {
    ctx, end := logx.Start(logx.HttpExtract(r.Context(), r), "handler")
    defer end()

    logx.Info(ctx, "received")
}
```

### 手动构造根 context

适用于无法通过函数参数或请求头传递的场景，例如消息队列消费、定时任务。

```go
// ID 可由上游传递，也可由本地生成
traceID := logx.GenTraceID()
spanID := logx.GenSpanID()

ctx, err := logx.NewRootContext(traceID, spanID)
if err != nil {
    return err
}

// 基于该 ctx 创建的 span 会挂在传入的 trace 下
ctx, end := logx.Start(ctx, "consume")
defer end()
```

> `NewRootContext` 构造的 context 会被标记为**已采样**，因此不受 `TraceSampleRatio` 影响、必定上报。它表达的语义是「这条 trace 我要追」，适合消息消费、定时任务这类明确需要留痕的入口；如果这类入口量很大，应在调用方自行决定是否要开 span。

## 优雅退出

追踪数据由 `BatchSpanProcessor` 批量异步导出，进程直接退出会丢失缓冲区中的 span。

```go
logx.Init(conf, "my-service")
defer logx.Shutdown(context.Background()) // 导出缓存并关闭 provider
```

| 方法 | 说明 |
| --- | --- |
| `Flush(ctx) error` | 立即导出缓存中的 span，不关闭 provider |
| `Shutdown(ctx) error` | 导出缓存中的 span 并关闭 provider |

两者在未开启追踪时均为空操作，直接返回 `nil`，调用方无需判断配置。

## 后端对接

### 本地 Jaeger

Jaeger 自 v1.35 起原生支持 OTLP 摄取，v1.49 及以上默认开启。

```yaml
services:
  jaeger:
    image: jaegertracing/all-in-one:1.57
    environment:
      - COLLECTOR_OTLP_ENABLED=true # v1.49+ 默认已开启
    ports:
      - "16686:16686" # UI
      - "4318:4318"   # OTLP/HTTP
```

```go
logx.Config{
    EnableTrace:      true,
    TraceSampleRatio: 1,
    OTLPEndpoint:     "localhost:4318",
    OLTPInsecure:     true, // 本地为纯 HTTP，必须开启
    // OTLPEndpointURLPath 留空即为 /v1/traces，与 Jaeger 一致
}
```

访问 `http://localhost:16686`，按 `Init` 传入的服务名检索即可。

### Loki + Tempo + Grafana

完整的 docker-compose 示例见 [`example/grafana`](./example/grafana)。

```go
logx.Config{
    Output:           "console",
    LokiServer:       "http://localhost:3100/loki/api/v1/push",
    EnableTrace:      true,
    TraceSampleRatio: 1,
    OTLPEndpoint:     "localhost:4318",
    OLTPInsecure:     true,
}
```

在 Grafana 的 Loki 数据源中配置 derived field，用 `trace_id` 指向 Tempo 数据源，即可从日志跳转到完整链路。

### Grafana Cloud

```go
logx.Config{
    EnableTrace:         true,
    TraceSampleRatio:    0.1,
    OTLPEndpoint:        "otlp-gateway-prod-ap-southeast-1.grafana.net",
    OTLPEndpointURLPath: "/otlp/v1/traces",
    OTLPToken:           "<base64(instanceID:token)>",
}
```

## API 参考

### 初始化与生命周期

| 函数 | 说明 |
| --- | --- |
| `Init(conf Config, serviceName string, attrs ...Field)` | 初始化。`serviceName` 会写入 span 的 `service.name` 与 Loki 的 `service_name` label |
| `Flush(ctx context.Context) error` | 立即导出缓存中的 span |
| `Shutdown(ctx context.Context) error` | 导出缓存并关闭 provider |

### 日志等级

| 函数 | 说明 |
| --- | --- |
| `SetLevel(level string)` | 运行时调整日志等级，并发安全 |
| `GetLevel() string` | 返回当前日志等级 |

### 日志记录

| 函数 | 说明 |
| --- | --- |
| `Debug(ctx, msg string, attrs ...Field)` | 调试日志 |
| `Info(ctx, msg string, attrs ...Field)` | 普通日志 |
| `Warn(ctx, msg string, attrs ...Field)` | 警告日志 |
| `Error(ctx, msg string, attrs ...Field)` | 错误日志，在 span 上记为 exception |
| `Fatal(ctx, msg string, attrs ...Field)` | 致命错误，记录后以退出码 1 结束进程 |

### 链路追踪

| 函数 | 说明 |
| --- | --- |
| `Start(ctx, spanName string, attrs ...Field) (context.Context, func())` | 创建 span，返回携带该 span 的 context 与结束函数 |
| `SetSpanAttr(ctx, attrs ...Field)` | 为当前 span 追加属性 |
| `TraceID(ctx) string` / `SpanID(ctx) string` | 获取当前 trace / span 的 ID |
| `GenTraceID() string` / `GenSpanID() string` | 生成合法的 trace / span ID |
| `NewRootContext(traceID, spanID string) (context.Context, error)` | 由指定 ID 构造根 context |

### 链路传递

| 函数 | 说明 |
| --- | --- |
| `HttpInject(ctx, req *http.Request) error` | 将 span 信息注入 HTTP 请求头 |
| `HttpExtract(ctx, req *http.Request) context.Context` | 从 HTTP 请求头提取上游 span 信息 |
| `TraceHeaders(ctx) map[string]string` | 导出当前 span 的追踪 header，未开启追踪时返回 `nil` |
| `ContextFromHeaders(ctx, headers map[string]string) context.Context` | 从追踪 header 还原 context |
| `GinMiddleware() gin.HandlerFunc` | Gin 中间件，提取上游 span 信息并写出 traceID 到 gin 上下文与响应头 |
| `TraceIDKey` | 常量 `"trace-id"`，gin 上下文 key 与响应头名 |

### Field 类型

```go
logx.Bool("ok", true)
logx.Int("age", 30)
logx.Int64("id", 1001)
logx.Float64("score", 99.5)
logx.String("name", "alice")
logx.Stringer("addr", addr)      // 任意实现 fmt.Stringer 的类型
logx.Any("payload", obj)         // 任意类型，在 span 属性中序列化为 JSON 字符串
logx.Err(err)                    // 固定使用 error 作为 key
```

`Bool` / `Int` / `Int64` / `Float64` / `String` 均提供对应的切片版本：`BoolSlice`、`IntSlice`、`Int64Slice`、`Float64Slice`、`StringSlice`。

## 注意事项

- **`Start` 返回的 `end` 必须以 `defer end()` 的形式直接调用**。缺少调用会导致 span 既不结束也不导出，并持续占用内存；写成 `defer func(){ end() }()` 会让 panic 记录失效，因为 `recover` 只在被 `defer` 的函数中直接调用才生效。
- **`end()` 不吞 panic**。它记录日志与 span 异常后会原样重新抛出，所以 panic 仍会向上传播——Web 服务应保留 `gin.Recovery()` 之类的兜底中间件，否则 panic 会导致进程退出。
- **`Init` 只应调用一次**。重复调用会关闭旧的 TracerProvider 再新建，期间缓冲区中未导出的 span 会被 flush，但并发调用不安全。
- **采样率会影响 span event**。`TraceSampleRatio` 设为 `0.1` 时，约 90% 的 span 及其 event 不会上报，而 Loki 推送不受采样影响、始终为全量。需要完整日志检索能力时应同时配置 Loki。
- **采样是 `ParentBased` 的**。上游已做出采样决策时直接沿用，`TraceSampleRatio` 只对根 span 生效。这样同一条 trace 不会因为各服务采样率不同而被割裂——上游采样了、下游却丢弃。反过来，上游若传来**未标记采样**的上下文（`traceparent` 的 flags 为 `00`，或 B3 头缺少 `X-B3-Sampled`），本服务会遵从该决策、不上报这条 trace。这是 OpenTelemetry 的标准行为，若上游服务未正确设置采样标志，会表现为下游「没有 trace」。
- **`TraceSampleRatio` 的零值不是「不采样」**。未设置（或取值超出 `(0,1]`）时按全量采样处理，否则 `EnableTrace: true` 却忘了设采样率会导致一条 trace 都没有。要关闭追踪请设 `EnableTrace: false`。
- **单个 span 的 event 数量有上限**。OpenTelemetry SDK 默认每个 span 最多 128 个 event，超出部分会被静默丢弃。长生命周期的 span 应拆分为多个子 span。
- **没有 span 的 ctx 不会产生 event**。未经 `Start` 的 context（如进程启动阶段、未传递 ctx 的定时任务）调用日志方法时，本地日志和 Loki 正常写入，但不会产生 span event。
- **`Fatal` 会结束进程**。它会先记录 span、推送 Loki、强制导出追踪数据，然后以退出码 `1` 退出，其后的代码不会执行。
- **传播格式同时支持 B3 与 W3C Trace Context**。注入时两种 header 都会写入（`b3` 与 `traceparent`），提取时两种格式都能识别，因此与只认其中一种格式的上下游均可对接。
- **`TracerProviderType` 配置了不支持的值会导致进程退出**（`log.Fatal`），仅接受 `oltp` 与 `file`。

## 效果预览

基于 Loki + Tempo + Grafana 的日志查询与链路追踪：

![日志查询与链路追踪](https://user-images.githubusercontent.com/5791324/147378026-42819fbb-5abf-46b9-8bba-a3ec7bfbcc81.png)

## License

本项目基于 [MIT License](./LICENSE) 开源。
