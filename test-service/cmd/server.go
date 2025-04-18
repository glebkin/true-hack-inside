package main

import (
	"context"
	"fmt"
	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"github.com/juju/zaputil/zapctx"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net"
	"os/signal"
	"runtime/debug"
	"syscall"
	"truetechhackllm/test-service/api"
	"truetechhackllm/test-service/internal/config"
	"truetechhackllm/test-service/internal/model"
	"truetechhackllm/test-service/internal/obs"
	"truetechhackllm/test-service/internal/obs/logger"
	"truetechhackllm/test-service/internal/storage"
	"truetechhackllm/test-service/internal/truetech"
)

func main() {
	log := logger.New(logger.WithConfig(logger.NewDefaultConfig()))

	ctx := zapctx.WithLogger(context.Background(), log)

	cfg, err := config.New()
	if err != nil {
		log.Fatal("failed to load config", zap.Error(err))
	}

	err = obs.InitTraceProvider(cfg.TraceCollector, "true-tech-server", log)
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

	if err = runServer(ctx, log, cfg); err != nil {
		log.Fatal("failed to run server app", zap.Error(err))
	}
}

func runServer(ctx context.Context, log *zap.Logger, cfg config.Config) error {
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	panicsTotal := promauto.NewCounter(prometheus.CounterOpts{
		Name: "grpc_req_panics_recovered_total",
		Help: "Total number of gRPC requests recovered from internal panic.",
	})
	grpcPanicRecoveryHandler := func(p any) (err error) {
		panicsTotal.Inc()
		log.Error("recovered from panic", zap.Any("p", p), zap.Any("stack", debug.Stack()))
		return status.Errorf(codes.Internal, "%s", p)
	}

	logTraceID := func(ctx context.Context) logging.Fields {
		if spanCtx := oteltrace.SpanContextFromContext(ctx); spanCtx.IsSampled() {
			return logging.Fields{"traceID", spanCtx.TraceID().String(), "spanID", spanCtx.SpanID().String()}
		}
		return nil
	}
	srvMetrics := grpcprom.NewServerMetrics(
		grpcprom.WithServerHandlingTimeHistogram(
			grpcprom.WithHistogramBuckets([]float64{0.001, 0.01, 0.1, 0.3, 0.6, 1, 3, 6, 9, 20, 30, 60, 90, 120}),
		),
	)
	prometheus.MustRegister(srvMetrics)
	exemplarFromContext := func(ctx context.Context) prometheus.Labels {
		if span := oteltrace.SpanContextFromContext(ctx); span.IsSampled() {
			return prometheus.Labels{"traceID": span.TraceID().String(), "spanID": span.SpanID().String()}
		}
		return nil
	}

	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler(
			otelgrpc.WithTracerProvider(otel.GetTracerProvider()),
			otelgrpc.WithPropagators(propagation.NewCompositeTextMapPropagator(
				propagation.TraceContext{}, propagation.Baggage{},
			)),
		)),
		grpc.ChainUnaryInterceptor(
			srvMetrics.UnaryServerInterceptor(grpcprom.WithExemplarFromContext(exemplarFromContext)),
			logging.UnaryServerInterceptor(obs.InterceptorLogger(log), logging.WithFieldsFromContext(logTraceID)),
			recovery.UnaryServerInterceptor(recovery.WithRecoveryHandler(grpcPanicRecoveryHandler)),
		),
	)

	service := truetech.NewService(storage.NewInMemory[string, model.Team]())
	api.RegisterTrueTechHackContestServer(grpcServer, service)
	srvMetrics.InitializeMetrics(grpcServer)

	lis, err := net.Listen("tcp", cfg.ServerAddress)
	if err != nil {
		return fmt.Errorf("creating tcp socket: %w", err)
	}

	log.Info("starting server", zap.String("SERVER_ADDRESS", cfg.ServerAddress))

	return grpcServer.Serve(lis)
}
