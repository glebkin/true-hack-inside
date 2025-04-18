package obs

import (
	"context"
	"fmt"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.uber.org/zap"
	"net/http"
	"net/http/pprof"
	"runtime"
	"time"
)

func InterceptorLogger(l *zap.Logger) logging.Logger {
	return logging.LoggerFunc(func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
		f := make([]zap.Field, 0, len(fields)/2)

		for i := 0; i < len(fields); i += 2 {
			key := fields[i]
			value := fields[i+1]

			switch v := value.(type) {
			case string:
				f = append(f, zap.String(key.(string), v))
			case int:
				f = append(f, zap.Int(key.(string), v))
			case bool:
				f = append(f, zap.Bool(key.(string), v))
			default:
				f = append(f, zap.Any(key.(string), v))
			}
		}

		logger := l.WithOptions(zap.AddCallerSkip(1)).With(f...)

		switch lvl {
		case logging.LevelDebug:
			logger.Debug(msg)
		case logging.LevelInfo:
			logger.Info(msg)
		case logging.LevelWarn:
			logger.Warn(msg)
		case logging.LevelError:
			logger.Error(msg)
		default:
			panic(fmt.Sprintf("unknown level %v", lvl))
		}
	})
}

func StartPprof(pprofAddr string, log *zap.Logger) error {
	log.Info("starting pprof server", zap.String("PPROF_ADDRESS", pprofAddr))

	mux := http.NewServeMux()

	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	runtime.SetBlockProfileRate(1)

	const readHeaderTimeout = 5 * time.Second

	srv := &http.Server{
		Addr:              pprofAddr,
		ReadHeaderTimeout: readHeaderTimeout,
		Handler:           mux,
	}

	return srv.ListenAndServe()
}

func StartPrometheusExporter(addr string, log *zap.Logger) error {
	log.Info("starting prometheus exporter", zap.String("PROMETHEUS_ADDRESS", addr))

	exporter, err := prometheus.New()
	if err != nil {
		return fmt.Errorf("creating prometheus exporter: %w", err)
	}
	provider := metric.NewMeterProvider(metric.WithReader(exporter))
	otel.SetMeterProvider(provider)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	const readHeaderTimeout = 5 * time.Second

	srv := &http.Server{
		Addr:              addr,
		ReadHeaderTimeout: readHeaderTimeout,
		Handler:           mux,
	}

	return srv.ListenAndServe()
}

func InitTraceProvider(addr, serviceName string, log *zap.Logger) error {
	log.Info("creating otel trace provider", zap.String("JAEGER_ADDRESS", addr))

	exporter, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(addr)))
	if err != nil {
		return fmt.Errorf("creating jaeger exporter: %w", err)
	}

	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		return err
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(r),
	)

	otel.SetTracerProvider(provider)

	return nil
}
