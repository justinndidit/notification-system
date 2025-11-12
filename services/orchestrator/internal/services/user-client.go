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

func NewUserClient(logger *zerolog.Logger, address string) *UserClient {
	return &UserClient{
		userAddress: address,
		baseClient:  NewBaseHTTPClient(logger),
	}
}

func (u *UserClient) FetchUserPreference(ctx context.Context, id string, wg *sync.WaitGroup, resultChan chan<- dtos.HTTPResponse) {
	defer wg.Done()

	url := fmt.Sprintf("%s/users/preference/%s", u.userAddress, id)

	u.baseClient.DoWithRetry(ctx, url, resultChan, "Failed to fetch user preferences")

}
