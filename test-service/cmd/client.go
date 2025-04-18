package main

import (
	"context"
	"fmt"
	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/timeout"
	"github.com/juju/zaputil/zapctx"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"os/signal"
	"syscall"
	"time"
	"truetechhackllm/test-service/api"
	"truetechhackllm/test-service/internal/client"
	"truetechhackllm/test-service/internal/config"
	"truetechhackllm/test-service/internal/obs"
	"truetechhackllm/test-service/internal/obs/logger"
)

func main() {
	log := logger.New(logger.WithConfig(logger.NewDefaultConfig()))

	ctx := zapctx.WithLogger(context.Background(), log)

	cfg, err := config.New()
	if err != nil {
		log.Fatal("failed to load config", zap.Error(err))
	}

	err = obs.InitTraceProvider(cfg.TraceCollector, "true-tech-client", log)
	if err != nil {
		log.Fatal("failed to initialize trace provider", zap.Error(err))
	}

	go func() {
		err = obs.StartPprof(cfg.PprofAddress, log)
		if err != nil {
			log.Fatal("failed to start pprof", zap.Error(err))
		}
	}()
	go func() {
		err = obs.StartPrometheusExporter(cfg.PrometheusAddress, log)
		if err != nil {
			log.Fatal("failed to start prometheus exporter", zap.Error(err))
		}
	}()

	if err = runClient(ctx, log, cfg); err != nil {
		log.Fatal("failed to run client app", zap.Error(err))
	}
}

func runClient(ctx context.Context, log *zap.Logger, cfg config.Config) error {
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logTraceID := func(ctx context.Context) logging.Fields {
		if spanCtx := oteltrace.SpanContextFromContext(ctx); spanCtx.IsSampled() {
			return logging.Fields{"traceID", spanCtx.TraceID().String(), "spanID", spanCtx.SpanID().String()}
		}
		return nil
	}
	clMetrics := grpcprom.NewClientMetrics(
		grpcprom.WithClientHandlingTimeHistogram(
			grpcprom.WithHistogramBuckets([]float64{0.001, 0.01, 0.1, 0.3, 0.6, 1, 3, 6, 9, 20, 30, 60, 90, 120}),
		),
	)
	prometheus.MustRegister(clMetrics)
	exemplarFromContext := func(ctx context.Context) prometheus.Labels {
		if span := oteltrace.SpanContextFromContext(ctx); span.IsSampled() {
			return prometheus.Labels{"traceID": span.TraceID().String(), "spanID": span.SpanID().String()}
		}
		return nil
	}

	cc, err := grpc.NewClient(
		cfg.ServerAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler(
			otelgrpc.WithTracerProvider(otel.GetTracerProvider()),
			otelgrpc.WithPropagators(propagation.NewCompositeTextMapPropagator(
				propagation.TraceContext{}, propagation.Baggage{},
			)),
		)),
		grpc.WithChainUnaryInterceptor(
			timeout.UnaryClientInterceptor(500*time.Millisecond),
			clMetrics.UnaryClientInterceptor(grpcprom.WithExemplarFromContext(exemplarFromContext)),
			logging.UnaryClientInterceptor(obs.InterceptorLogger(log), logging.WithFieldsFromContext(logTraceID)),
		),
	)
	if err != nil {
		return fmt.Errorf("creating grpc client: %w", err)
	}

	worker := client.NewWorker(api.NewTrueTechHackContestClient(cc), cfg.ClientInterval)

	log.Info("starting client")

	return worker.Start(ctx)
}
