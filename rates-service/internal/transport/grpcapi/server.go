package grpcapi

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/evgenypakhomov/bank-system-go/rates-service/internal/domain"
	ratesv1 "github.com/evgenypakhomov/bank-system-go/rates-service/internal/gen/rates/v1"
)

type Server struct {
	ratesv1.UnimplementedRateServiceServer
	service RateService
}

type RateService interface {
	Get(context.Context, string) (domain.ExchangeRateResponse, error)
}

func Register(registrar grpc.ServiceRegistrar, service RateService) {
	ratesv1.RegisterRateServiceServer(registrar, &Server{service: service})
}

func (s *Server) GetRate(ctx context.Context, request *ratesv1.GetRateRequest) (*ratesv1.GetRateResponse, error) {
	response, err := s.service.Get(ctx, request.GetCurrency())
	if err != nil {
		return nil, err
	}
	result := &ratesv1.GetRateResponse{Success: response.Success, Currency: response.Currency}
	if response.ErrorMessage != nil {
		result.ErrorMessage = *response.ErrorMessage
	}
	if response.RateToRUB != nil {
		result.RateToRub = response.RateToRUB.String()
	}
	if response.Timestamp != nil {
		result.Timestamp = timestamppb.New(*response.Timestamp)
	}
	return result, nil
}
