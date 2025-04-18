package client

import (
	"context"
	"fmt"
	"github.com/juju/zaputil/zapctx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"math/rand"
	"time"
	"truetechhackllm/test-service/api"
)

type Worker struct {
	grpcClient api.TrueTechHackContestClient

	tickInterval time.Duration
	tracer       trace.Tracer
	rand         *rand.Rand
}

func NewWorker(grpcClient api.TrueTechHackContestClient, interval time.Duration) *Worker {
	return &Worker{
		grpcClient:   grpcClient,
		tickInterval: interval,
		tracer:       otel.Tracer("true-tech-client-worker"),
		rand:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

var teams = map[string][]string{
	"best-team": {"member-1", "member-2", "member-3"},
}

func (w *Worker) Start(ctx context.Context) error {
	log := zapctx.Logger(ctx)

	for ticker := time.NewTicker(w.tickInterval); ; {
		err := w.do(ctx)
		if err != nil {
			log.Error("something went wrong during doing workers job", zap.Error(err))
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("worker finished due to context cancellation: %w", ctx.Err())
		case <-ticker.C:
			continue
		}
	}
}

func (w *Worker) do(ctx context.Context) error {
	log := zapctx.Logger(ctx)
	spanCtx, span := w.tracer.Start(ctx, "worker.Do")
	defer span.End()

	chance := w.rand.Float64()
	createTeam := w.rand.Intn(2)

	switch {
	case chance < 0.1:
		log.Info("registering team")

		var teamName string
		var members []string

		for key, value := range teams {
			teamName = key
			members = value
		}

		if createTeam == 1 {
			teams["team-"+time.Now().String()] = []string{"member-" + time.Now().String()}
		}

		span.SetAttributes(attribute.KeyValue{
			Key:   "team",
			Value: attribute.StringValue(teamName),
		})

		req := &api.RegisterTeamRequest{
			TeamName:    teamName,
			TeamMembers: members,
		}

		_, err := w.grpcClient.RegisterTeam(spanCtx, req)
		if err != nil {
			log.Error("calling RegisterTeam", zap.Error(err))
		}
	case chance < 0.4:
		log.Info("submitting solution")

		var teamName string

		for key := range teams {
			teamName = key
		}

		span.SetAttributes(attribute.KeyValue{
			Key:   "team",
			Value: attribute.StringValue(teamName),
		})

		req := &api.SubmitSolutionRequest{
			TeamName: teamName,
			Solution: "best-solution",
		}
		_, err := w.grpcClient.SubmitSolution(spanCtx, req)
		if err != nil {
			log.Error("calling SubmitSolution", zap.Error(err))
		}
	default:
		log.Info("getting leaderboard")

		resp, err := w.grpcClient.GetLeaderboard(spanCtx, &api.GetLeaderboardRequest{})
		if err != nil {
			log.Error("calling GetLeaderboard", zap.Error(err))
		} else {
			log.Info("got response", zap.Any("response", resp))
		}
	}

	return nil
}
