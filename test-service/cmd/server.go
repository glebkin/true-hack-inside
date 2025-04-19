package main

import (
	"context"
	"fmt"
	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/juju/zaputil/zapctx"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"math/rand/v2"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"
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

	err = obs.InitTraceProvider(ctx, cfg.TraceCollector, "true-tech-server", log)
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

	controlInt := &controlInterceptor{r: rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))}
	go controlInt.start(log, cfg.DebugControlUrl)

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
			controlInt.Intercept,
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

type controlInterceptor struct {
	mustPanic bool
	mustDelay bool
	mustError bool

	r *rand.Rand
}

func (i *controlInterceptor) start(log *zap.Logger, addr string) error {
	log.Info("starting control interceptor server", zap.String("DEBUG_CONTROL_URL", addr))
	const readHeaderTimeout = 5 * time.Second

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/control", i.handle)
	srv := &http.Server{
		Addr:              addr,
		ReadHeaderTimeout: readHeaderTimeout,
		Handler:           mux,
	}

	return srv.ListenAndServe()
}

func (i *controlInterceptor) handle(w http.ResponseWriter, r *http.Request) {
	queryParams := r.URL.Query()

	panicParam := queryParams.Get("panic")
	if panicParam != "" {
		panicValue, err := parseBool(panicParam)
		if err != nil {
			http.Error(w, "Invalid value for panic parameter", http.StatusBadRequest)
			return
		}
		i.mustPanic = panicValue
	}

	delayParam := queryParams.Get("delay")
	if delayParam != "" {
		delayValue, err := parseBool(delayParam)
		if err != nil {
			http.Error(w, "Invalid value for delay parameter", http.StatusBadRequest)
			return
		}
		i.mustDelay = delayValue
	}

	errorParam := queryParams.Get("error")
	if errorParam != "" {
		errorValue, err := parseBool(errorParam)
		if err != nil {
			http.Error(w, "Invalid value for error parameter", http.StatusBadRequest)
			return
		}
		i.mustError = errorValue
	}
}

func parseBool(value string) (bool, error) {
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean value")
	}
}

func (i *controlInterceptor) Intercept(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
	log := zapctx.Logger(ctx)

	if i.mustPanic {
		log.Panic("mustPanic passed")
	}
	if i.mustDelay {
		delay := time.Duration(rand.IntN(900)+100) * time.Millisecond
		log.Info("mustDelay called", zap.Duration("delay", delay))
		time.Sleep(delay)
	}
	if i.mustError {
		log.Error("mustError passed")
		return nil, status.Errorf(codes.Internal, "must error")
	}

	return handler(ctx, req)
}
