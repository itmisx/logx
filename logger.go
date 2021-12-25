package logger

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"time"

	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Config
type Config struct {
	// Print info or error,default false
	Debug bool `yaml:"debug" mapstructure:"debug"`
	// Enable to save log to the disk,default false
	EnableLog bool `yaml:"enable_log" mapstructure:"enable_log"`
	// Enable to generate trace info,default false
	EnableTrace bool `yaml:"enable_trace" mapstructure:"enable_trace"`
	// Path to store the log,default ./run.log
	File string `yaml:"file" mapstructure:"file"`
	// MaxSize is the maximum size in megabytes of the log file before it gets
	// rotated. It defaults to 100 megabytes.
	MaxSize int `yaml:"max_size" mapstructure:"max_size"`
	// MaxBackups is the maximum number of old log files to retain.  The default
	// is to retain all old log files (though MaxAge may still cause them to get
	// deleted.)
	MaxBackups int `yaml:"max_backups" mapstructure:"max_backups"`
	// MaxAge is the maximum number of days to retain old log files based on the
	// timestamp encoded in their filename.  Note that a day is defined as 24
	// hours and may not exactly correspond to calendar days due to daylight
	// savings, leap seconds, etc. The default is not to remove old log files
	// based on age.
	MaxAge int `yaml:"max_age" mapstructure:"max_age"`
	// Compress determines if the rotated log files should be compressed
	// using gzip. The default is not to perform compression.
	Compress bool `yaml:"compress" mapstructure:"compress"`
	// "* * * * * *"，The smallest unit is seconds，refer to linux crond format
	// Rotate causes Logger to close the existing log file and immediately create a new one.
	// This is a helper function for applications that want to initiate rotations outside of the normal rotation rules,
	// such as in response to SIGHUP. After rotating,
	// this initiates a cleanup of old log files according to the normal rules.
	Rotate string `yaml:"rotate" mapstructure:"rotate"`
	// TraceProviderType support file or jaeger
	TracerProviderType string `yaml:"tracer_provider_type" mapstructure:"tracer_provider_type"`
	// trace sampling, 0.0-1
	// 0,never trace
	// 1,always trace
	TraceSampleRatio float64 `yaml:"trace_sample_ratio" mapstructure:"trace_sample_ratio"`
	// Jaeger URI
	JaegerServer   string `yaml:"jaeger_server" mapstructure:"jaeger_server"`
	JaegerUsername string `yaml:"jaeger_username" mapstructure:"jaeger_username"`
	JaegerPassword string `yaml:"jaeger_password" mapstructure:"jaeger_password"`
}

var (
	logger   *zap.Logger
	config   Config
	provider *trace.TracerProvider
)

// save context span
type LoggerSpanContext struct {
	span       oteltrace.Span
	startSpan  bool
	errorCount int
}

type loggerSpanContext int

const loggerSpanContextKey loggerSpanContext = iota

// init
// auto init,this can useful to use zap/log directly
func init() {
	zapLogger := newZapLogger(Config{})
	logger = zapLogger.Logger
}

// LoggerInit logger init
// applicationAttributes
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
	zapLogger := newZapLogger(conf)
	logger = zapLogger.Logger
	// crond, rotate the log
	zapLogger.rotateCrond(conf)
}

// Start start span
//
// example 1:
// parentCtx:=Start(context.Background(),"span1",String("spanAttr","attr"))
// Info(parentCtx,"msg",String("Attr","attr"))
// End(parentCtx)
// childCtx:=Start(parentCtx,"span2")
// Info(childCtx,"msg")
// End(childCtx)
//
// example 2:
// ctx:= NewRootContext(traceID,spanID)
// ctx1:=Start(ctx,"span1",String("spanAttr","attr"))
// Info(ctx1,"msg")
// End(ctx1)
func Start(ctx context.Context, spanName string, spanStartOption ...Field) context.Context {
	var loggerSpanContext LoggerSpanContext
	var spanContext context.Context
	var span oteltrace.Span
	loggerSpanContext.startSpan = true
	spanName = spanName + " | " + time.Now().Format("15:04:05")
	if config.EnableTrace {
		spanContext, span = provider.Tracer("").Start(ctx, spanName, oteltrace.WithAttributes(FieldsToKeyValues(spanStartOption...)...))
		loggerSpanContext.span = span
	} else {
		// if trace disabled,create a noopTrace
		spanContext, span = oteltrace.NewNoopTracerProvider().Tracer("").Start(ctx, spanName)
		loggerSpanContext.span = span
	}
	ctx = context.WithValue(spanContext, loggerSpanContextKey, loggerSpanContext)
	return ctx
}

// Debug record debug
func Debug(ctx context.Context, msg string, attributes ...Field) {
	logger.Debug(msg, FieldsToZapFields(ctx, attributes...)...)
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
	logger.Info(msg, FieldsToZapFields(ctx, attributes...)...)
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
	logger.Warn(msg, FieldsToZapFields(ctx, attributes...)...)
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

// Error record error
func Error(ctx context.Context, msg string, attributes ...Field) {
	logger.Error(msg, FieldsToZapFields(ctx, attributes...)...)
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
	logger.Fatal(msg, FieldsToZapFields(ctx, attributes...)...)
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
}

func Flush(ctx context.Context) {
	provider.Shutdown(ctx)
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
		case BoolType:
			kvs = append(kvs, zap.Bool(f.Key, f.Bool))
		case BoolSliceType:
			kvs = append(kvs, zap.Bools(f.Key, f.Bools))
		case IntType:
			kvs = append(kvs, zap.Int(f.Key, f.Integer))
		case IntSliceType:
			kvs = append(kvs, zap.Ints(f.Key, f.Integers))
		case Int64Type:
			kvs = append(kvs, zap.Int64(f.Key, f.Integer64))
		case Int64SliceType:
			kvs = append(kvs, zap.Int64s(f.Key, f.Integer64s))
		case Float64Type:
			kvs = append(kvs, zap.Float64(f.Key, f.Float64))
		case Float64SliceType:
			kvs = append(kvs, zap.Float64s(f.Key, f.Float64s))
		case StringType:
			kvs = append(kvs, zap.String(f.Key, f.String))
		case StringSliceType:
			kvs = append(kvs, zap.Strings(f.Key, f.Strings))
		case StringerType:
			kvs = append(kvs, zap.String(f.Key, f.String))
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
