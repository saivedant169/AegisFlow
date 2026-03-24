package telemetry

import (
	"context"
	"log"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

var tracer trace.Tracer

func Init(serviceName, exporter string) (func(), error) {
	var sp sdktrace.SpanExporter
	var err error

	switch exporter {
	case "stdout":
		sp, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
	default:
		sp, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
	}

	if err != nil {
		return nil, err
	}

	res, err := resource.New(context.Background(),
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(sp),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	tracer = tp.Tracer(serviceName)

	log.Printf("telemetry initialized (exporter: %s)", exporter)

	shutdown := func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Printf("telemetry shutdown error: %v", err)
		}
	}

	return shutdown, nil
}

func Tracer() trace.Tracer {
	if tracer == nil {
		return otel.Tracer("aegisflow")
	}
	return tracer
}
