package truetech

import (
	"context"
	"fmt"
	"math/rand/v2"
	"truetechhackllm/test-service/api"
	"truetechhackllm/test-service/internal/model"

	"go.opentelemetry.io/otel"
	oteltrace "go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Service struct {
	api.UnimplementedTrueTechHackContestServer

	teamStorage teamStorage
	tracer      oteltrace.Tracer
}

func NewService(storage teamStorage) *Service {
	return &Service{
		teamStorage: storage,
		tracer:      otel.Tracer("true-tech-server-service"),
	}
}

func (s *Service) RegisterTeam(ctx context.Context, req *api.RegisterTeamRequest) (*emptypb.Empty, error) {
	_, span := s.tracer.Start(ctx, "Server.RegisterTeam")
	defer span.End()

	if req.GetTeamName() == "" {
		return nil, status.Error(codes.InvalidArgument, "team name is required")
	}
	if len(req.GetTeamMembers()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "team members is required")
	}

	_, found := s.teamStorage.Get(req.GetTeamName())
	if found {
		return nil, status.Errorf(codes.AlreadyExists, "team %s already exists", req.GetTeamName())
	}

	s.teamStorage.Put(req.GetTeamName(), model.Team{
		Name:     req.GetTeamName(),
		Members:  req.GetTeamMembers(),
		Score:    0,
		Solution: "",
	})

	return &emptypb.Empty{}, nil
}

func (s *Service) SubmitSolution(ctx context.Context, req *api.SubmitSolutionRequest) (*emptypb.Empty, error) {
	_, span := s.tracer.Start(ctx, "Server.SubmitSolution")
	defer span.End()

	team, found := s.teamStorage.Get(req.TeamName)
	if !found {
		return nil, status.Errorf(codes.NotFound, "team %s not found", req.TeamName)
	}
	team.Solution = req.Solution
	s.teamStorage.Put(req.TeamName, team)

	return &emptypb.Empty{}, nil
}

func (s *Service) GetLeaderboard(ctx context.Context, _ *api.GetLeaderboardRequest) (*api.GetLeaderboardResponse, error) {
	_, span := s.tracer.Start(ctx, "Server.GetLeaderboard")
	defer span.End()

	resp := &api.GetLeaderboardResponse{
		Scores: nil,
	}

	// Randomly inject error ~20% of the time
	if rand.Float64() < 0.2 {
		return nil, fmt.Errorf("injected random error")
	}
	teams := s.teamStorage.GetAll()
	for _, team := range teams {
		resp.Scores = append(resp.Scores, &api.TeamScore{
			TeamName: team.Name,
			Score:    team.Score,
		})
	}

	return resp, nil
}

type teamStorage interface {
	Get(name string) (model.Team, bool)
	Put(name string, team model.Team)
	GetAll() []model.Team
}
