package logx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"regexp"
	"runtime"
	"strconv"
	"time"

	"github.com/imroc/req/v3"
	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Config 配置项
type Config struct {
	// 是否开启debug模式，未开启debug模式，仅记录错误
	Debug bool `yaml:"debug" mapstructure:"debug"`
	// 日志输出的方式
	// none为不输出日志，file 为文件方式输出，console为控制台。默认为none
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
	// Loki配置
	// 一种是直接配置
	// 一种是在docker中安装插件，并配置容器的log loki选项，由插件自动完成推送
	LokiServer   string `yaml:"loki_server" mapstructure:"loki_server"`
	LokiUsername string `yaml:"loki_username" mapstructure:"loki_username"`
	LokiPassword string `yaml:"loki_password" mapstructure:"loki_password"`
	// 追踪使能
	EnableTrace bool `yaml:"enable_trace" mapstructure:"enable_trace"`
	// 日志追踪的类型，file/oltp，默认oltp
	TracerProviderType string `yaml:"tracer_provider_type" mapstructure:"tracer_provider_type"`
	// 日志追踪采样的比率, 0.0-1
	// 0,never trace
	// 1,always trace
	TraceSampleRatio float64 `yaml:"trace_sample_ratio" mapstructure:"trace_sample_ratio"`
	// 默认使用https，为false时，使用http
	OLTPInsecure bool `yaml:"oltp_insecure" mapstructure:"oltp_insecure"`
	// oltp endpoint 将trace data发送到该地址
	OTLPEndpoint        string `yaml:"oltp_endpoint" mapstructure:"oltp_endpoint"`
	OTLPEndpointURLPath string `yaml:"oltp_endpoint_url_path" mapstructure:"oltp_endpoint_url_path"`
	// 用户basic auth
	OTLPToken string `yaml:"oltp_token" mapstructure:"oltp_token"`
}

var (
	enable_log bool
	logger     *zap.Logger
	config     Config
	provider   *trace.TracerProvider
	reqClient  *req.Client
)

var LokiLabel = map[string]string{}

// save context span
type LoggerSpanContext struct {
	span oteltrace.Span
}

type LoggerContextKey int

const (
	loggerSpanContextKey LoggerContextKey = iota
)

// LoggerInit logger初始化
//
// applicationAttributes 应用属性，如应用的名称，版本等
// 可以使用 service.name,service.namesapce,service.instance.id,service.version,
// telemetry.sdk.name,telemetry.sdk.language,telemetry.sdk.version,telemetry.auto.version
// or other key that you need 主要用于追踪label
//
// 日志的label不支持*.*格式，会被过滤掉
//
// example:
// Init(conf,String("sevice.name",service1))
func Init(conf Config, serviceName string, applicationAttributes ...Field) {
	otel.SetTextMapPropagator(b3.New())
	config = conf
	// 设置loki的label
	var reg = regexp.MustCompile(`^[0-9A-Za-z_]+$`)
	LokiLabel["service_name"] = serviceName
	for _, attr := range applicationAttributes {
		if attr.Type == stringType && reg.MatchString(attr.Key) {
			LokiLabel[attr.Key] = attr.String
		}
	}
	if config.LokiServer != "" {
		reqClient = req.C().SetCommonBasicAuth(config.LokiUsername, config.LokiPassword)
	}
	if config.EnableTrace {
		var pd *trace.TracerProvider
		if conf.TracerProviderType == "" {
			conf.TracerProviderType = "oltp"
		}
		switch conf.TracerProviderType {
		case "oltp":
			pd, _ = Trace{}.NewOLTPProvider(context.Background(), conf, serviceName, applicationAttributes...)
		case "file":
			pd, _ = Trace{}.NewFileProvider(conf, serviceName, applicationAttributes...)
		default:
			log.Fatal("Unsupported tracerProvider type")
		}

		if pd != nil {
			provider = pd
		}
	}
	// 默认不输出日志
	if config.Output == "" {
		config.Output = "none"
	}
	if config.Output != "none" {
		enable_log = true
		if config.Output != "file" && config.Output != "console" {
			config.Output = "console"
		}
		zapLogger := newZapLogger(conf)
		logger = zapLogger.Logger
		zapLogger.rotateCrond(conf)
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
	// 根据配置开启日志追踪
	if config.EnableTrace {
		enableTrace = true
	}
	spanName = spanName + " | " + time.Now().Format("15:04:05")
	// 根据条件
	// 如果未开启追踪，则返回一个nooptreace，意味着将不再追踪
	if enableTrace {
		spanContext, span = provider.Tracer("").Start(ctx, spanName, oteltrace.WithAttributes(FieldsToKeyValues(spanStartOption...)...))
		loggerSpanContext.span = span
	} else {
		// 如果trace失能，将会创建一个noop traceProvider
		spanContext, span = noop.NewTracerProvider().Tracer("").Start(ctx, spanName)
		loggerSpanContext.span = span
	}
	startCtx := context.WithValue(spanContext, loggerSpanContextKey, loggerSpanContext)
	return startCtx
}

// SetSpanAttr 为当前的span动态设置属性
func SetSpanAttr(ctx context.Context, attributes ...Field) {
	loggerSpanContext, ok := ctx.Value(loggerSpanContextKey).(LoggerSpanContext)
	if !ok {
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
	if config.LokiServer != "" {
		lokiPush(ctx, "debug", msg, attributes...)
	}
	loggerSpanContext, ok := ctx.Value(loggerSpanContextKey).(LoggerSpanContext)
	if !ok {
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
	if config.LokiServer != "" {
		lokiPush(ctx, "info", msg, attributes...)
	}
	loggerSpanContext, ok := ctx.Value(loggerSpanContextKey).(LoggerSpanContext)
	if !ok {
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
	if config.LokiServer != "" {
		lokiPush(ctx, "warn", msg, attributes...)
	}
	loggerSpanContext, ok := ctx.Value(loggerSpanContextKey).(LoggerSpanContext)
	if !ok {
		return
	}
	if config.EnableTrace {
		loggerSpanContext.span.AddEvent(msg, oteltrace.WithAttributes(FieldsToKeyValues(attributes...)...))
	}
}

// Error record error
func Error(ctx context.Context, msg string, attributes ...Field) {
	if enable_log {
		logger.Error(msg, FieldsToZapFields(ctx, attributes...)...)
	}
	if config.LokiServer != "" {
		lokiPush(ctx, "error", msg, attributes...)
	}
	loggerSpanContext, ok := ctx.Value(loggerSpanContextKey).(LoggerSpanContext)
	if !ok {
		return
	}
	if config.EnableTrace {
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
		if config.EnableTrace {
			// add error logs
			loggerSpanContext.span.RecordError(errors.New(msg), oteltrace.WithAttributes(FieldsToKeyValues(attributes...)...))
		}
	}()
	if enable_log {
		logger.Fatal(msg, FieldsToZapFields(ctx, attributes...)...)
	}
	if config.LokiServer != "" {
		lokiPush(ctx, "fatal", msg, attributes...)
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
	if config.EnableTrace {
		loggerSpanContext.span.End()
	}
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

func lokiPush(ctx context.Context, level, msg string, attributes ...Field) {
	if reqClient == nil {
		return
	}
	var kv = map[string]interface{}{}
	// 日志等级
	kv["level"] = level
	// 日志标题
	kv["msg"] = msg
	// 日志追踪信息
	if traceID := TraceID(ctx); traceID != "" {
		kv["trace_id"] = traceID
	}
	if spanID := SpanID(ctx); spanID != "" {
		kv["span_id"] = spanID
	}
	for _, attr := range attributes {
		switch attr.Type {
		case boolType:
			kv[attr.Key] = attr.Bool
		case boolSliceType:
			kv[attr.Key] = attr.Bools
		case intType:
			kv[attr.Key] = attr.Integer
		case intSliceType:
			kv[attr.Key] = attr.Integers
		case int64Type:
			kv[attr.Key] = attr.Integer64
		case int64SliceType:
			kv[attr.Key] = attr.Integer64s
		case float64Type:
			kv[attr.Key] = attr.Float64
		case float64SliceType:
			kv[attr.Key] = attr.Float64s
		case stringType:
			kv[attr.Key] = attr.String
		case stringSliceType:
			kv[attr.Key] = attr.Strings
		case stringerType:
			kv[attr.Key] = attr.String
		case anyType:
			kv[attr.Key] = attr.Any
		}
	}
	// 获取调用堆栈信息
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		fmt.Println("无法获取调用信息")
		return
	}
	kv["caller"] = fmt.Sprintf("%s:%d", file, line)
	kvJson, _ := json.Marshal(kv)
	var data = map[string][]map[string]interface{}{
		"streams": {
			{
				"stream": LokiLabel,
				"values": [][]interface{}{{strconv.FormatInt(time.Now().UnixNano(), 10), string(kvJson)}},
			},
		},
	}
	jsonBytes, _ := json.Marshal(data)
	go func() {
		reqCtx, reqCancel := context.WithTimeout(context.Background(), time.Second*3)
		defer reqCancel()
		reqClient.
			R().
			SetContext(reqCtx).
			SetHeader("content-type", "application/json").
			SetBodyJsonBytes(jsonBytes).
			Post(config.LokiServer)
	}()
}
