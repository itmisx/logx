package logger

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"sync"
	"time"

	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Config 配置项
type Config struct {
	// 是否开启debug模式，未开启debug模式，仅记录错误
	Debug bool `yaml:"debug" mapstructure:"debug"`
	// 追踪使能
	EnableTrace bool `yaml:"enable_trace" mapstructure:"enable_trace"`
	// 日志输出的方式
	// none为不输出日志，file 为文件方式输出，console为控制台。默认为console
	Output string `yaml:"output" mapstructure:"output"`
	// 日志文件路径
	File string `yaml:"file" mapstructure:"file"` // 日志文件路径
	// 日志文件大小限制，默认最大100MB,超过将触发文件切割
	MaxSize int `yaml:"max_size" mapstructure:"max_size"`
	// 日志文件的分割文件的数量，超过的将会被删除
	MaxBackups int `yaml:"max_backups" mapstructure:"max_backups"`
	// 日志文件的保留时间，超过的将会被删除
	MaxAge int `yaml:"max_age" mapstructure:"max_age"`
	// 是否启用日志文件的压缩功能
	Compress bool `yaml:"compress" mapstructure:"compress"`
	// 是否启用日志的切割功能
	Rotate string `yaml:"rotate" mapstructure:"rotate"`
	// 日志追踪的类型，file or jaeger
	TracerProviderType string `yaml:"tracer_provider_type" mapstructure:"tracer_provider_type"`
	// 日志追踪采样的比率, 0.0-1
	// 0,never trace
	// 1,always trace
	TraceSampleRatio float64 `yaml:"trace_sample_ratio" mapstructure:"trace_sample_ratio"`
	// 最大span数量限制，当达到最大限制时，停止trace
	// default 200
	MaxSpan int `yaml:"max_span" mapstructure:"max_span"`
	// Jaeger server
	JaegerServer   string `yaml:"jaeger_server" mapstructure:"jaeger_server"`
	JaegerUsername string `yaml:"jaeger_username" mapstructure:"jaeger_username"`
	JaegerPassword string `yaml:"jaeger_password" mapstructure:"jaeger_password"`
}

var (
	enable_log bool
	logger     *zap.Logger
	config     Config
	provider   *trace.TracerProvider
)

// save context span
type LoggerSpanContext struct {
	span       oteltrace.Span
	startSpan  bool
	warnCount  int
	errorCount int
}

type loggerSpanContext int

const loggerSpanContextKey loggerSpanContext = iota

type spanCountT struct {
	count sync.Map
}

// span数量控制
var spanCount = spanCountT{
	count: sync.Map{},
}

// 获取span计数
func (s *spanCountT) get(traceID string, spanID string) int {
	key := _md5(traceID + spanID)
	c, ok := s.count.Load(key)
	if ok {
		count, _ := c.(int)
		return count
	}
	return 0
}

// 设置span计数
func (s *spanCountT) set(traceID string, spanID string, count int) {
	key := _md5(traceID + spanID)
	s.count.Store(key, count)
}

// span 计数自增
func (s *spanCountT) increase(traceID string, spanID string) {
	oldCount := s.get(traceID, spanID)
	s.set(traceID, spanID, oldCount+1)
}

func (s *spanCountT) delete(traceID string, spanID string) {
	key := _md5(traceID + spanID)
	s.count.Delete(key)
}

// LoggerInit logger初始化
//
// applicationAttributes 应用属性，如应用的名称，版本等
// can use service.name,service.namesapce,service.instance.id,service.version,
// telemetry.sdk.name,telemetry.sdk.language,telemetry.sdk.version,telemetry.auto.version
// or other key that you need
//
// example:
// Init(conf,String("sevice.name",service1))
func Init(conf Config, applicationAttributes ...Field) {
	otel.SetTextMapPropagator(b3.New())
	config = conf
	if config.EnableTrace {
		var pd *trace.TracerProvider
		if conf.TracerProviderType == "" {
			conf.TracerProviderType = "jaeger"
		}
		if conf.TracerProviderType == "jaeger" {
			pd, _ = Trace{}.NewJaegerProvider(conf, applicationAttributes...)
		} else if conf.TracerProviderType == "file" {
			pd, _ = Trace{}.NewFileProvider(conf, applicationAttributes...)
		} else {
			log.Fatal("Unsupported tracerProvider type")
		}

		if pd != nil {
			provider = pd
		}
	}
	if config.Output != "none" {
		enable_log = true
		if config.Output == "" || (config.Output != "file" && config.Output != "console") {
			config.Output = "console"
		}
		zapLogger := newZapLogger(conf)
		logger = zapLogger.Logger
		zapLogger.rotateCrond(conf)
	}
	if config.MaxSpan == 0 {
		config.MaxSpan = 200
	}
}

// Start 启动一个span追踪
// ctx 上级span
// spanName span名字
// spanStartOption span附带属性
func Start(ctx context.Context, spanName string, spanStartOption ...Field) context.Context {
	var loggerSpanContext LoggerSpanContext
	var spanContext context.Context
	var enableTrace bool
	var span oteltrace.Span
	// 如果超过最大跟踪span限制，则停止追踪
	if spanCount.get(TraceID(ctx), SpanID(ctx)) >= config.MaxSpan {
		enableTrace = false
	} else {
		if config.EnableTrace {
			enableTrace = true
		}
	}
	loggerSpanContext.startSpan = true
	spanName = spanName + " | " + time.Now().Format("15:04:05")
	// 根据条件
	// 如果未开启追踪，则返回一个nooptreace，意味着将不再追踪
	if enableTrace {
		spanContext, span = provider.Tracer("").Start(ctx, spanName, oteltrace.WithAttributes(FieldsToKeyValues(spanStartOption...)...))
		loggerSpanContext.span = span
		spanCount.increase(TraceID(ctx), SpanID(ctx))
	} else {
		// 如果trace失能，将会创建一个noop traceProvider
		spanContext, span = oteltrace.NewNoopTracerProvider().Tracer("").Start(ctx, spanName)
		loggerSpanContext.span = span
	}
	return context.WithValue(spanContext, loggerSpanContextKey, loggerSpanContext)
}

// SetSpanAttr 为当前的span动态设置属性
func SetSpanAttr(ctx context.Context, attributes ...Field) {
	loggerSpanContext, ok := ctx.Value(loggerSpanContextKey).(LoggerSpanContext)
	if !ok {
		return
	}
	if !loggerSpanContext.startSpan {
		return
	}
	if config.EnableTrace {
		loggerSpanContext.span.SetAttributes(FieldsToKeyValues(attributes...)...)
	}
}

// Debug record debug
func Debug(ctx context.Context, msg string, attributes ...Field) {
	if enable_log {
		logger.Debug(msg, FieldsToZapFields(ctx, attributes...)...)
	}
	loggerSpanContext, ok := ctx.Value(loggerSpanContextKey).(LoggerSpanContext)
	if !ok {
		return
	}
	if !loggerSpanContext.startSpan {
		return
	}
	if config.EnableTrace {
		loggerSpanContext.span.AddEvent(msg, oteltrace.WithAttributes(FieldsToKeyValues(attributes...)...))
	}
}

// Info record info
func Info(ctx context.Context, msg string, attributes ...Field) {
	if enable_log {
		logger.Info(msg, FieldsToZapFields(ctx, attributes...)...)
	}
	loggerSpanContext, ok := ctx.Value(loggerSpanContextKey).(LoggerSpanContext)
	if !ok {
		return
	}
	if !loggerSpanContext.startSpan {
		return
	}
	if config.EnableTrace {
		loggerSpanContext.span.AddEvent(msg, oteltrace.WithAttributes(FieldsToKeyValues(attributes...)...))
	}
}

// Warn record warn
func Warn(ctx context.Context, msg string, attributes ...Field) {
	if enable_log {
		logger.Warn(msg, FieldsToZapFields(ctx, attributes...)...)
	}
	loggerSpanContext, ok := ctx.Value(loggerSpanContextKey).(LoggerSpanContext)
	if !ok {
		return
	}
	if !loggerSpanContext.startSpan {
		return
	}
	if config.EnableTrace {
		// add error tag and amount
		loggerSpanContext.warnCount++
		loggerSpanContext.span.SetAttributes(attribute.Int("warns", loggerSpanContext.warnCount))
		loggerSpanContext.span.AddEvent(msg, oteltrace.WithAttributes(FieldsToKeyValues(attributes...)...))
	}
}

// Error record error
func Error(ctx context.Context, msg string, attributes ...Field) {
	if enable_log {
		logger.Error(msg, FieldsToZapFields(ctx, attributes...)...)
	}
	loggerSpanContext, ok := ctx.Value(loggerSpanContextKey).(LoggerSpanContext)
	if !ok {
		return
	}
	if !loggerSpanContext.startSpan {
		return
	}
	if config.EnableTrace {
		// add error tag and amount
		loggerSpanContext.errorCount++
		loggerSpanContext.span.SetAttributes(attribute.Int("errors", loggerSpanContext.errorCount))
		// add error logs
		loggerSpanContext.span.RecordError(errors.New(msg), oteltrace.WithAttributes(FieldsToKeyValues(attributes...)...))
	}
}

// Fatal record fatal
func Fatal(ctx context.Context, msg string, attributes ...Field) {
	defer func() {
		loggerSpanContext, ok := ctx.Value(loggerSpanContextKey).(LoggerSpanContext)
		if !ok {
			return
		}
		if !loggerSpanContext.startSpan {
			return
		}
		if config.EnableTrace {
			// add error logs
			loggerSpanContext.span.RecordError(errors.New(msg), oteltrace.WithAttributes(FieldsToKeyValues(attributes...)...))
		}
	}()
	if enable_log {
		logger.Fatal(msg, FieldsToZapFields(ctx, attributes...)...)
	}
}

// TraceID return traceID
func TraceID(ctx context.Context) string {
	loggerSpanContext, ok := ctx.Value(loggerSpanContextKey).(LoggerSpanContext)
	if !ok {
		return ""
	}
	return loggerSpanContext.span.SpanContext().TraceID().String()
}

// TraceID return traceID
func SpanID(ctx context.Context) string {
	loggerSpanContext, ok := ctx.Value(loggerSpanContextKey).(LoggerSpanContext)
	if !ok {
		return ""
	}
	return loggerSpanContext.span.SpanContext().SpanID().String()
}

// NewRootContext new root context with given traceID and spanID
func NewRootContext(traceID string, spanID string) (context.Context, error) {
	var (
		err error
		tID oteltrace.TraceID
		sID oteltrace.SpanID
		ctx = context.Background()
	)
	// set traceID and spanID
	{
		tID, err = oteltrace.TraceIDFromHex(traceID)
		if err != nil {
			return ctx, errors.New("invalid traceID")
		}
		sID, err = oteltrace.SpanIDFromHex(spanID)
		if err != nil {
			return ctx, errors.New("invalid spanID")
		}
	}
	// generate root ctx by spanContextConfig
	sc := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID: tID,
		SpanID:  sID,
	})
	ctx = oteltrace.ContextWithRemoteSpanContext(ctx, sc)
	return ctx, nil
}

// GenTraceID generate traceID
// current timestamp with 8 rand bytes
func GenTraceID() string {
	stime := strconv.FormatInt(time.Now().UnixNano(), 16)
	paddingLen := 16 - len(stime)
	for i := 0; i < paddingLen; i++ {
		stime = stime + "0"
	}
	return stime + randString(16)
}

// GenSpanID gererate spanID
func GenSpanID() string {
	return randString(16)
}

// End end trace
func End(ctx context.Context) {
	if err := recover(); err != nil {
		logger.Error("panic", zap.String("recover", fmt.Sprint(err)), zap.Stack("stack"))
	}
	loggerSpanContext, ok := ctx.Value(loggerSpanContextKey).(LoggerSpanContext)
	if !ok {
		return
	}
	loggerSpanContext.startSpan = false
	if config.EnableTrace {
		loggerSpanContext.span.End()
	}
	spanCount.delete(TraceID(ctx), SpanID(ctx))
}

// FieldsToZapFields
func FieldsToZapFields(ctx context.Context, fields ...Field) []zapcore.Field {
	kvs := []zapcore.Field{}
	if traceID := TraceID(ctx); traceID != "" {
		kvs = append(kvs, zap.String("trace_id", traceID))
	}
	if spanID := SpanID(ctx); spanID != "" {
		kvs = append(kvs, zap.String("span_id", spanID))
	}
	for _, f := range fields {
		switch f.Type {
		case boolType:
			kvs = append(kvs, zap.Bool(f.Key, f.Bool))
		case boolSliceType:
			kvs = append(kvs, zap.Bools(f.Key, f.Bools))
		case intType:
			kvs = append(kvs, zap.Int(f.Key, f.Integer))
		case intSliceType:
			kvs = append(kvs, zap.Ints(f.Key, f.Integers))
		case int64Type:
			kvs = append(kvs, zap.Int64(f.Key, f.Integer64))
		case int64SliceType:
			kvs = append(kvs, zap.Int64s(f.Key, f.Integer64s))
		case float64Type:
			kvs = append(kvs, zap.Float64(f.Key, f.Float64))
		case float64SliceType:
			kvs = append(kvs, zap.Float64s(f.Key, f.Float64s))
		case stringType:
			kvs = append(kvs, zap.String(f.Key, f.String))
		case stringSliceType:
			kvs = append(kvs, zap.Strings(f.Key, f.Strings))
		case stringerType:
			kvs = append(kvs, zap.String(f.Key, f.String))
		case anyType:
			kvs = append(kvs, zap.Any(f.Key, f.Any))
		}
	}
	return kvs
}

// RandString 生成随机字符串
func randString(len int) string {
	bytes := []byte{}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	seed := "0123456789abcdef"
	for i := 0; i < len; i++ {
		bytes = append(bytes, seed[r.Intn(15)])
	}
	return string(bytes)
}

// Md5 Md5
func _md5(str string) string {
	plain := md5.New()
	plain.Write([]byte(str))
	return hex.EncodeToString(plain.Sum(nil))
}
