package logx

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/itmisx/logx/propagation/extract"
	b3prop "go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// tracePropagator 链路传递统一使用的propagator
//
// 同时支持b3和w3c traceparent两种格式：
// 注入时两种header都会写入，提取时两种格式都能识别，
// 以便与只认其中一种格式的上下游服务对接
//
// 所有的注入/提取都走这个变量，不依赖全局propagator，
// 避免被otel.SetTextMapPropagator覆盖后行为不一致
var tracePropagator propagation.TextMapPropagator = propagation.NewCompositeTextMapPropagator(
	propagation.TraceContext{},
	propagation.Baggage{},
	b3prop.New(),
)

// TraceIDKey traceID在gin上下文中的key，同时也是响应头的名字
//
// GinMiddleware会自动写入，应用层可直接取用：
// c.GetString(logx.TraceIDKey)
const TraceIDKey = extract.TraceIDKey

// HttpInject 将当前span的追踪信息注入http请求头
func HttpInject(ctx context.Context, request *http.Request) error {
	if request == nil {
		return errors.New("nil request")
	}
	if !oteltrace.SpanContextFromContext(ctx).IsValid() {
		return nil
	}
	tracePropagator.Inject(ctx, propagation.HeaderCarrier(request.Header))
	return nil
}

// HttpExtract 从http请求头提取上游的追踪信息，与HttpInject对应
//
// 返回的context携带的是远端spanContext，还不是可记录日志的span，
// 需要再调用Start，生成的span即为上游span的子span
func HttpExtract(ctx context.Context, request *http.Request) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if request == nil {
		return ctx
	}
	return tracePropagator.Extract(ctx, propagation.HeaderCarrier(request.Header))
}

// TraceHeaders 返回当前span对应的追踪header，用于手动传递
//
// 适用于无法使用HttpInject的场景，如消息队列、grpc metadata、
// 或自行封装的http客户端
//
// 未开启追踪时span为noop，traceID和spanID全为0，
// 此时返回nil，避免向下游发送无效的追踪header
//
// 返回的key为http规范化形式（如X-B3-Traceid），
// 用于grpc metadata等要求小写key的场景时需自行转换
//
// example:
//
//	for k, v := range logx.TraceHeaders(ctx) {
//	    msg.Header.Set(k, v)
//	}
func TraceHeaders(ctx context.Context) map[string]string {
	if !oteltrace.SpanContextFromContext(ctx).IsValid() {
		return nil
	}
	header := http.Header{}
	tracePropagator.Inject(ctx, propagation.HeaderCarrier(header))
	headers := make(map[string]string, len(header))
	for key := range header {
		headers[key] = header.Get(key)
	}
	return headers
}

// ContextFromHeaders 从追踪header中提取上游的追踪信息并生成context
//
// 与TraceHeaders对应，b3和w3c traceparent两种格式都能识别，
// key大小写不敏感
//
// 返回的context携带的是远端spanContext，还不是可记录日志的span，
// 需要再调用Start，生成的span即为上游span的子span
//
// headers中没有有效的追踪信息时，原样返回传入的ctx，
// 此时Start会创建一条新的trace
//
// example:
// ctx := logx.ContextFromHeaders(context.Background(), headers)
// ctx, end := logx.Start(ctx, "consume")
// defer end()
func ContextFromHeaders(ctx context.Context, headers map[string]string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(headers) == 0 {
		return ctx
	}
	header := make(http.Header, len(headers))
	for key, value := range headers {
		header.Set(key, value)
	}
	return tracePropagator.Extract(ctx, propagation.HeaderCarrier(header))
}

// GinMiddleware 提取上游span信息并创建server span
//
// 服务名由Init提供，已写入span的resource，中间件无需再传：
// 底层的http.server_name语义是「匹配到的虚拟主机名」而非服务名，
// 填服务名既重复又不符合语义，故这里固定留空
//
// 确实需要设置虚拟主机名时，直接使用
// github.com/itmisx/logx/propagation/extract 下的 GinMiddleware(service, opts...)
func GinMiddleware() gin.HandlerFunc {
	return extract.GinMiddleware("", extract.WithPropagators(tracePropagator))
}
