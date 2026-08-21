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

func NewTemplateClient(logger *zerolog.Logger, address, serviceToken string) *TemplateClient {
	return &TemplateClient{
		clientAddress: address,
		baseClient: NewBaseHTTPClient(logger, map[string]string{
			"X-Service-Token": serviceToken,
		}),
	}
}

func (t *TemplateClient) FetchTemplateById(ctx context.Context, id string, wg *sync.WaitGroup, resultChan chan<- dtos.HTTPResponse) {
	defer wg.Done()

	url := fmt.Sprintf("%s/template/%s", t.clientAddress, id)

	t.baseClient.DoWithRetry(ctx, url, resultChan, "Failed to fetch template")
}

// RenderTemplate compiles the template's latest version against the supplied
// context. Rendering happens here, during enrichment, so that every worker
// receives finished content and no channel has to implement a template engine.
func (t *TemplateClient) RenderTemplate(ctx context.Context, id string, renderContext map[string]any) (dtos.HTTPResponse, error) {
	url := fmt.Sprintf("%s/template/%s/render", t.clientAddress, id)

	return t.baseClient.PostJSON(ctx, url, map[string]any{
		"data": renderContext,
	})
}
