package logx

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/itmisx/logx/propagation/extract"
	"github.com/itmisx/logx/propagation/inject"
)

// HTTPInject inject spanContext
func HttpInject(ctx context.Context, request *http.Request) error {
	return inject.HttpInject(ctx, request)
}

// GinMiddleware extract spanContext
func GinMiddleware(service string) gin.HandlerFunc {
	return extract.GinMiddleware(service)
}
