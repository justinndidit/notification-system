package services

import (
	"context"
	"fmt"
	"sync"

	"github.com/justinndidit/notificationSystem/orchestrator/internal/dtos"
	"github.com/rs/zerolog"
)

type UserClient struct {
	baseClient  *BaseHTTPClient
	userAddress string
}

func NewUserClient(logger *zerolog.Logger, address string, tokenProvider func() (string, error)) *UserClient {
	return &UserClient{
		userAddress: address,
		baseClient:  NewBaseHTTPClient(logger, tokenProvider),
	}
}

func (u *UserClient) FetchDeliveryProfile(ctx context.Context, id string, wg *sync.WaitGroup, resultChan chan<- dtos.HTTPResponse) {
	defer wg.Done()

	url := fmt.Sprintf("%s/user/%s/delivery-profile", u.userAddress, id)

	u.baseClient.DoWithRetry(ctx, url, resultChan, "Failed to fetch user delivery profile")

}
