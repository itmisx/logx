module github.com/itmisx/logger

go 1.16

require (
	github.com/BurntSushi/toml v0.4.1 // indirect
	github.com/gin-gonic/gin v1.7.7
	github.com/robfig/cron/v3 v3.0.1
	github.com/stretchr/testify v1.7.0
	go.opentelemetry.io/contrib/propagators/b3 v1.3.0
	go.opentelemetry.io/otel v1.3.0
	go.opentelemetry.io/otel/exporters/jaeger v1.2.0
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.2.0
	go.opentelemetry.io/otel/sdk v1.2.0
	go.opentelemetry.io/otel/trace v1.3.0
	go.uber.org/zap v1.19.1
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
)
