package logx

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/itmisx/logx"
	"github.com/itmisx/logx/propagation/extract"
)

func TestTrace(*testing.T) {
	conf := logx.Config{
		Debug:               true,
		Output:              "console",
		LokiServer:          "",
		LokiUsername:        "",
		LokiPassword:        "",
		EnableTrace:         true,
		TraceSampleRatio:    1,
		OTLPEndpoint:        "otlp-gateway-prod-ap-southeast-1.grafana.net",
		OTLPEndpointURLPath: "/otlp/v1/traces",
		OTLPToken:           "",
	}

	logx.Init(conf, logx.Int64("ID", 1), logx.String("service.name", "local-test"))
	ctx1 := logx.Start(context.Background(), "test1", logx.String("service.name", "local-test"))
	logx.Warn(ctx1, "test info", logx.String("name1", "test"))
	logx.Error(ctx1, "test info", logx.Bool("sex1", true))
	logx.Error(ctx1, "test info", logx.Int("age1", 30))

	ctx2 := logx.Start(ctx1, "test2", logx.Int("spanNum", 2))
	fmt.Println(logx.TraceID(ctx2))
	logx.Error(ctx2, "test2 info", logx.String("name2", "test"))
	logx.Info(ctx2, "test2 info", logx.Bool("sex2", true))
	logx.Debug(ctx2, "test2 info", logx.Any("conf", conf))
	logx.End(ctx2)
	logx.End(ctx1)
	time.Sleep(time.Second * 7)
}

func TestCustomizeTraceID(*testing.T) {
	conf := logx.Config{
		Debug:              true,
		EnableTrace:        true,
		Output:             "file",
		File:               "./run.log",
		Rotate:             "0 * * * * *",
		TracerProviderType: "file",
	}
	logx.Init(conf, logx.Int64("ID", 1))
	traceID := logx.GenTraceID()
	spanID := logx.GenSpanID()
	ctx, err := logx.NewRootContext(traceID, spanID)
	if err != nil {
		fmt.Println(err)
	}
	ctx = logx.Start(ctx, "test1")
	logx.Warn(ctx, "test info", logx.String("name1", "test"), logx.Any("conf", conf))
	logx.Error(ctx, "test info", logx.Bool("sex1", true))
	logx.Error(ctx, "test info", logx.Int("age1", 30))
	logx.End(ctx)

	<-make(chan bool)
}

func TestLog(*testing.T) {
	conf := logx.Config{
		Debug:  true,
		Output: "file",
		Rotate: "0 * * * * *",
	}
	ctx := context.Background()
	logx.Init(conf)
	for {
		logx.Info(ctx, "log info")
		logx.Warn(ctx, "log warn")
		logx.Error(ctx, "log error")
		time.Sleep(time.Second * 5)
	}
}

func TestPropagationWithGlobalPropagators(t *testing.T) {
	// logx init
	{
		conf := logx.Config{
			Debug:              true,
			EnableTrace:        true,
			TracerProviderType: "file",
			TraceSampleRatio:   1,
		}
		logx.Init(conf, logx.Int64("ID", 1))
	}

	// new gin server
	{
		router := gin.Default()
		router.Static("/upload", "./upload") // 上传目录
		router.Use(extract.GinMiddleware(""))
		router.GET("/user/:id", func(c *gin.Context) {
			ctx := logx.Start(c.Request.Context(), "123")
			logx.Info(ctx, "456")
			logx.End(ctx)
		})
		router.POST("/open-api/control/signal-test", func(c *gin.Context) {
			ctx := logx.Start(c.Request.Context(), "123")
			logx.Info(ctx, "789")
			logx.End(ctx)
		})
		go func() {
			router.Run()
		}()
	}

	// wait gin startup
	{
		time.Sleep(time.Second * 1)
	}

	// test propagation
	{
		ctx := logx.Start(context.Background(), "123")
		logx.Info(ctx, "123")
		logx.End(ctx)

		request, _ := http.NewRequest("GET", "http://localhost:8080/test", nil)
		logx.HttpInject(ctx, request)
		client := &http.Client{}
		client.Do(request)
	}

	// wait
	<-make(chan bool)
}

func TestMaxspan(*testing.T) {
	conf := logx.Config{
		Debug:              true,
		EnableTrace:        true,
		TracerProviderType: "file",
	}

	logx.Init(conf, logx.Int64("ID", 1))
	ctx1 := logx.Start(context.Background(), "test1")
	defer logx.End(ctx1)
	logx.Warn(ctx1, "test info", logx.String("name1", "test"))
	logx.Error(ctx1, "test info", logx.Bool("sex1", true))
	logx.Error(ctx1, "test info", logx.Int("age1", 30))
	for i := 0; i < 300; i++ {
		ctx2 := logx.Start(ctx1, "test2", logx.Int("spanNum", 2))
		fmt.Println(logx.TraceID(ctx2))
		logx.Error(ctx2, "test2 info", logx.String("name2", "test"))
		logx.Info(ctx2, "test2 info", logx.Bool("sex2", true))
		logx.Info(ctx2, "test2 info", logx.Int("age2", 30))
		logx.End(ctx2)
	}
}
