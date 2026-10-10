package inject

import (
	"context"
	"net/http"

	b3prop "go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/otel/propagation"
)

// propagator 同时支持b3和w3c traceparent
//
// 注意：不要在这里调用otel.SetTextMapPropagator，
// 那会在每次注入时修改全局状态，既有数据竞争又会影响其他组件的提取行为
var propagator = propagation.NewCompositeTextMapPropagator(
	propagation.TraceContext{},
	propagation.Baggage{},
	b3prop.New(),
)

// HttpInject 将当前span的追踪信息注入http请求头
func HttpInject(ctx context.Context, request *http.Request) error {
	if request == nil {
		return nil
	}
	propagator.Inject(ctx, propagation.HeaderCarrier(request.Header))
	return nil
}
