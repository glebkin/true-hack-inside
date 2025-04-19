package collector

import (
	"context"
	"errors"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"io"
	"time"

	"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	"go.uber.org/zap"
)

type JaegerCollector struct {
	logger *zap.Logger

	client api_v2.QueryServiceClient
}

func NewJaegerCollector(url string, logger *zap.Logger) (*JaegerCollector, error) {
	cc, err := grpc.NewClient(
		url,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	return &JaegerCollector{
		client: api_v2.NewQueryServiceClient(cc),
		logger: logger,
	}, nil
}

func (c *JaegerCollector) Collect(ctx context.Context, start, end time.Time) ([]string, error) {
	resp, err := c.client.GetServices(ctx, &api_v2.GetServicesRequest{})
	if err != nil {
		return nil, fmt.Errorf("get services: %w", err)
	}

	var result []string
	for _, service := range resp.GetServices() {
		traces, err := c.findTraces(ctx, service, start, end)
		if err != nil {
			return nil, fmt.Errorf("find traces for service %s: %w", service, err)
		}
		result = append(result, traces...)
	}

	return result, nil
}

func (c *JaegerCollector) findTraces(ctx context.Context, serviceName string, start, end time.Time) ([]string, error) {
	stream, err := c.client.FindTraces(ctx, &api_v2.FindTracesRequest{
		Query: &api_v2.TraceQueryParameters{
			ServiceName:  serviceName,
			StartTimeMin: start,
			StartTimeMax: end,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("find traces: %w", err)
	}

	var result []string
	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("find traces stream receive: %w", err)
		}

		for _, span := range resp.Spans {
			str := fmt.Sprintf("Trace: [ServiceName=%s;TraceID=%s;SpanID=%s;Duration=%s;StartTime=%s;ProcessID=%s;OperationName=%s]",
				serviceName, span.TraceID.String(), span.SpanID.String(), span.Duration.String(), span.StartTime.String(), span.ProcessID, span.OperationName)
			result = append(result, str)
		}
	}

	return result, nil
}
