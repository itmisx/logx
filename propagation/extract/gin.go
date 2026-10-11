package extract

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const (
	tracerKey  = "otel-go-contrib-tracer"
	tracerName = "go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	// TraceIDKey traceID在gin上下文中的key，同时也是响应头的名字
	TraceIDKey = "trace-id"
)

// Middleware returns middleware that will trace incoming requests.
// The service parameter should describe the name of the (virtual)
// server handling the request.
func GinMiddleware(service string, opts ...Option) gin.HandlerFunc {
	cfg := config{}
	for _, opt := range opts {
		opt.apply(&cfg)
	}
	if cfg.TracerProvider == nil {
		cfg.TracerProvider = otel.GetTracerProvider()
	}
	tracer := cfg.TracerProvider.Tracer(
		tracerName,
		// oteltrace.WithInstrumentationVersion(SemVersion()),
	)
	if cfg.Propagators == nil {
		cfg.Propagators = otel.GetTextMapPropagator()
	}
	return func(c *gin.Context) {
		c.Set(tracerKey, tracer)
		savedCtx := c.Request.Context()
		defer func() {
			c.Request = c.Request.WithContext(savedCtx)
		}()
		ctx := cfg.Propagators.Extract(savedCtx, propagation.HeaderCarrier(c.Request.Header))
		opts := []oteltrace.SpanStartOption{
			oteltrace.WithAttributes(semconv.NetAttributesFromHTTPRequest("tcp", c.Request)...),
			oteltrace.WithAttributes(semconv.HTTPServerAttributesFromHTTPRequest(service, c.FullPath(), c.Request)...),
			oteltrace.WithSpanKind(oteltrace.SpanKindServer),
		}
		spanName := c.FullPath()
		if spanName == "" {
			spanName = fmt.Sprintf("HTTP %s route not found", c.Request.Method)
		}
		ctx, span := tracer.Start(ctx, spanName, opts...)
		// panic会跳过c.Next()之后的代码，所以收尾统一放在defer里
		// 否则panic时server span不带任何状态，在trace视图中看起来是正常结束的
		//
		// panic分支不设置http.status_code：此时gin.Recovery还未写入响应，
		// c.Writer.Status()仍是默认的200，记录下来反而是错的
		defer func() {
			if err := recover(); err != nil {
				span.RecordError(fmt.Errorf("panic: %v", err), oteltrace.WithStackTrace(true))
				span.SetStatus(codes.Error, fmt.Sprint(err))
				span.End()
				// 原样抛出，交回gin.Recovery等上层处理
				panic(err)
			}
			status := c.Writer.Status()
			span.SetAttributes(semconv.HTTPAttributesFromHTTPStatusCode(status)...)
			spanStatus, spanMessage := semconv.SpanStatusFromHTTPStatusCode(status)
			span.SetStatus(spanStatus, spanMessage)
			if len(c.Errors) > 0 {
				span.SetAttributes(attribute.String("gin.errors", c.Errors.String()))
			}
			span.End()
		}()

		// 将traceID写入gin上下文和响应头
		// 应用层可直接 c.GetString(TraceIDKey)，调用方也能从响应头拿到
		// 必须在c.Next()之前写，否则响应已提交，header不再生效
		if traceID := span.SpanContext().TraceID(); traceID.IsValid() {
			c.Set(TraceIDKey, traceID.String())
			c.Header(TraceIDKey, traceID.String())
		}

		// pass the span through the request context
		c.Request = c.Request.WithContext(ctx)

		// serve the request to the next middleware
		c.Next()
	}
}
