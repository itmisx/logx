package inject

import (
	"context"
	"net/http"

	b3prop "go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

func HttpInject(ctx context.Context, request *http.Request) error {
	otel.SetTextMapPropagator(b3prop.New())
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(request.Header))
	return nil
}
