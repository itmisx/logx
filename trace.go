package logger

import (
	"encoding/json"
	"io"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

type Trace struct{}

// NewJaegerProvider
func (tx Trace) NewJaegerProvider(conf Config,
	attributes ...Field,
) (*trace.TracerProvider, error) {
	exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(conf.JaegerServer),
		jaeger.WithUsername(conf.JaegerUsername),
		jaeger.WithPassword(conf.JaegerPassword)))
	if err != nil {
		return nil, err
	}
	if conf.TraceSampleRatio > 1 {
		conf.TraceSampleRatio = 1
	}
	if conf.TraceSampleRatio < 0 {
		conf.TraceSampleRatio = 0
	}
	tp := trace.NewTracerProvider(
		// Always be sure to batch in production.
		trace.WithBatcher(exp),
		// Record information about this application in an Resource.
		trace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			FieldsToKeyValues(attributes...)...,
		)),
		trace.WithSampler(trace.TraceIDRatioBased(conf.TraceSampleRatio)),
	)

	otel.SetTracerProvider(tp)
	return tp, nil
}

// NewFileProvider
func (tx Trace) NewFileProvider(conf Config, attributes ...Field) (*trace.TracerProvider, error) {
	f, _ := os.Create("trace.txt")
	exp, _ := newExporter(f)
	tp := trace.NewTracerProvider(
		// Always be sure to batch in production.
		trace.WithBatcher(exp),
		// Record information about this application in an Resource.
		trace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			FieldsToKeyValues(attributes...)...,
		)),
		trace.WithSampler(trace.AlwaysSample()),
	)

	otel.SetTracerProvider(tp)
	return tp, nil
}

func newExporter(w io.Writer) (trace.SpanExporter, error) {
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
