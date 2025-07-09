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
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

type Trace struct{}

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
		sdktrace.WithSampler(
			sdktrace.TraceIDRatioBased(conf.TraceSampleRatio), // 没父 span 的时候按 10 % 随机采样
		),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			FieldsToKeyValues(attributes...)...,
		)),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp, err
}

// NewFileProvider
func (tx Trace) NewFileProvider(conf Config, serviceName string, attributes ...Field) (*sdktrace.TracerProvider, error) {
	f, _ := os.Create("trace.txt")
	exp, _ := newExporter(f)
	attributes = append(attributes, String("service.name", serviceName))
	tp := sdktrace.NewTracerProvider(
		// Always be sure to batch in production.
		sdktrace.WithBatcher(exp),
		// Record information about this application in an Resource.
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			FieldsToKeyValues(attributes...)...,
		)),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
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
