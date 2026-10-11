package logx

import (
	"context"
	"encoding/json"
	"io"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

type Trace struct{}

// newSampler 构造采样器
//
// 用ParentBased包裹：上游已有采样决策时直接沿用，
// 只有根span才按ratio决定。否则同一条trace会在服务间被割裂——
// 上游采样了、下游因ratio较小而丢弃，trace就断了
//
// ratio不在(0,1]范围内时视为未设置，按全量采样处理；
// 要完全关闭追踪请使用Config.EnableTrace，而不是把ratio设为0
func newSampler(ratio float64) sdktrace.Sampler {
	if ratio <= 0 || ratio > 1 {
		ratio = 1
	}
	return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))
}

// NewOLTPProvider
func (tx Trace) NewOLTPProvider(
	ctx context.Context,
	conf Config,
	serviceName string,
	attributes ...Field,
) (*sdktrace.TracerProvider, error) {
	var options []otlptracehttp.Option
	if conf.OTLPEndpoint != "" {
		options = append(options, otlptracehttp.WithEndpoint(conf.OTLPEndpoint))
	}
	if conf.OTLPEndpointURLPath != "" {
		options = append(options, otlptracehttp.WithURLPath(conf.OTLPEndpointURLPath))
	}
	if conf.OLTPInsecure {
		options = append(options, otlptracehttp.WithInsecure())
	}
	if conf.OTLPToken != "" {
		options = append(options, otlptracehttp.WithHeaders(map[string]string{
			"Authorization": "Basic " + conf.OTLPToken,
		}))
	}
	// the collected spans.
	exporter, err := otlptrace.New(context.Background(), otlptracehttp.NewClient(
		options...,
	))
	if err != nil {
		return nil, err
	}

	attributes = append(attributes, String("service.name", serviceName))
	// For the demonstration, use sdktrace.AlwaysSample sampler to sample all traces.
	// In a production application, use sdktrace.ProbabilitySampler with a desired probability.
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(newSampler(conf.TraceSampleRatio)),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			FieldsToKeyValues(attributes...)...,
		)),
	)
	otel.SetTracerProvider(tp)
	// propagator统一在Init中设置，这里不再覆盖
	// 否则会丢掉b3格式，导致HttpInject注入的header无法被提取
	return tp, err
}

// NewFileProvider
func (tx Trace) NewFileProvider(conf Config, serviceName string, attributes ...Field) (*sdktrace.TracerProvider, error) {
	// 不能吞掉这两个错误：工作目录只读时os.Create会失败，
	// 继续下去会返回一个看似健康、实则每次导出都失败的provider
	f, err := os.Create("trace.txt")
	if err != nil {
		return nil, err
	}
	exp, err := newExporter(f)
	if err != nil {
		return nil, err
	}
	attributes = append(attributes, String("service.name", serviceName))
	tp := sdktrace.NewTracerProvider(
		// Always be sure to batch in production.
		sdktrace.WithBatcher(exp),
		// Record information about this application in an Resource.
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			FieldsToKeyValues(attributes...)...,
		)),
		sdktrace.WithSampler(newSampler(conf.TraceSampleRatio)),
	)

	otel.SetTracerProvider(tp)
	return tp, nil
}

func newExporter(w io.Writer) (sdktrace.SpanExporter, error) {
	return stdouttrace.New(
		stdouttrace.WithWriter(w),
		// Use human-readable output.
		stdouttrace.WithPrettyPrint(),
		// Do not print timestamps for the demo.
		stdouttrace.WithoutTimestamps(),
	)
}

// FieldsToKeyValue
func FieldsToKeyValues(fields ...Field) []attribute.KeyValue {
	kvs := []attribute.KeyValue{}
	for _, f := range fields {
		switch f.Type {
		case boolType:
			kvs = append(kvs, attribute.Bool(f.Key, f.Bool))
		case boolSliceType:
			kvs = append(kvs, attribute.BoolSlice(f.Key, f.Bools))
		case intType:
			kvs = append(kvs, attribute.Int(f.Key, f.Integer))
		case intSliceType:
			kvs = append(kvs, attribute.IntSlice(f.Key, f.Integers))
		case int64Type:
			kvs = append(kvs, attribute.Int64(f.Key, f.Integer64))
		case int64SliceType:
			kvs = append(kvs, attribute.Int64Slice(f.Key, f.Integer64s))
		case float64Type:
			kvs = append(kvs, attribute.Float64(f.Key, f.Float64))
		case float64SliceType:
			kvs = append(kvs, attribute.Float64Slice(f.Key, f.Float64s))
		case stringType:
			kvs = append(kvs, attribute.String(f.Key, f.String))
		case stringSliceType:
			kvs = append(kvs, attribute.StringSlice(f.Key, f.Strings))
		case stringerType:
			stringer := stringer{str: f.String}
			kvs = append(kvs, attribute.Stringer(f.Key, stringer))
		case anyType:
			if str, err := json.Marshal(f.Any); err == nil {
				kvs = append(kvs, attribute.String(f.Key, string(str)))
			}
		}
	}
	return kvs
}

// stringer fmt.Stringer
type stringer struct {
	str string
}

func (s stringer) String() string {
	return s.str
}
