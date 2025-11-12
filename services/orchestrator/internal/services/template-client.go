package services

import (
	"context"
	"fmt"
	"sync"

	"github.com/justinndidit/notificationSystem/orchestrator/internal/dtos"
	"github.com/rs/zerolog"
)

type TemplateClient struct {
	clientAddress string
	baseClient    *BaseHTTPClient
}

func NewTemplateClient(logger *zerolog.Logger, address string) *TemplateClient {
	return &TemplateClient{
		clientAddress: address,
		baseClient:    NewBaseHTTPClient(logger),
	}
}

func (t *TemplateClient) FetchTemplateById(ctx context.Context, id string, wg *sync.WaitGroup, resultChan chan<- dtos.HTTPResponse) {
	defer wg.Done()

	url := fmt.Sprintf("%s/template/%s", t.clientAddress, id)

	t.baseClient.DoWithRetry(ctx, url, resultChan, "Failed to fetch template")
}
