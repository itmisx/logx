package logger

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/itmisx/logger"
	"github.com/itmisx/logger/propagation/extract"
)

func TestTrace(*testing.T) {
	conf := logger.Config{
		Debug:              true,
		EnableTrace:        true,
		TracerProviderType: "file",
	}

	logger.Init(conf, logger.Int64("ID", 1))
	ctx1 := logger.Start(context.Background(), "test1")
	defer logger.End(ctx1)
	logger.Warn(ctx1, "test info", logger.String("name1", "test"))
	logger.Error(ctx1, "test info", logger.Bool("sex1", true))
	logger.Error(ctx1, "test info", logger.Int("age1", 30))

	ctx2 := logger.Start(ctx1, "test2", logger.Int("spanNum", 2))
	fmt.Println(logger.TraceID(ctx2))
	logger.Error(ctx2, "test2 info", logger.String("name2", "test"))
	logger.Info(ctx2, "test2 info", logger.Bool("sex2", true))
	logger.Info(ctx2, "test2 info", logger.Int("age2", 30))
	logger.End(ctx2)
}

func TestCustomizeTraceID(*testing.T) {
	conf := logger.Config{
		Debug:              true,
		EnableTrace:        true,
		Output:             "file",
		File:               "./run.log",
		Rotate:             "0 * * * * *",
		TracerProviderType: "file",
	}
	logger.Init(conf, logger.Int64("ID", 1))
	traceID := logger.GenTraceID()
	spanID := logger.GenSpanID()
	ctx, err := logger.NewRootContext(traceID, spanID)
	if err != nil {
		fmt.Println(err)
	}
	ctx = logger.Start(ctx, "test1")
	logger.Warn(ctx, "test info", logger.String("name1", "test"), logger.Any("conf", conf))
	logger.Error(ctx, "test info", logger.Bool("sex1", true))
	logger.Error(ctx, "test info", logger.Int("age1", 30))
	logger.End(ctx)

	<-make(chan bool)
}

func TestLog(*testing.T) {
	conf := logger.Config{
		Debug:  true,
		Output: "file",
		Rotate: "0 * * * * *",
	}
	ctx := context.Background()
	logger.Init(conf)
	for {
		logger.Info(ctx, "log info")
		logger.Warn(ctx, "log warn")
		logger.Error(ctx, "log error")
		time.Sleep(time.Second * 5)
	}
}

func TestPropagationWithGlobalPropagators(t *testing.T) {
	// logger init
	{
		conf := logger.Config{
			Debug:              true,
			EnableTrace:        true,
			TracerProviderType: "file",
			TraceSampleRatio:   1,
			JaegerServer:       "http://120.77.213.80:14268/api/traces",
		}
		logger.Init(conf, logger.Int64("ID", 1))
	}

	// new gin server
	{
		router := gin.Default()
		router.Static("/upload", "./upload") // 上传目录
		router.Use(extract.GinMiddleware(""))
		router.GET("/user/:id", func(c *gin.Context) {
			ctx := logger.Start(c.Request.Context(), "123")
			logger.Info(ctx, "456")
			logger.End(ctx)
		})
		router.POST("/open-api/control/signal-test", func(c *gin.Context) {
			ctx := logger.Start(c.Request.Context(), "123")
			logger.Info(ctx, "789")
			logger.End(ctx)
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
		ctx := logger.Start(context.Background(), "123")
		logger.Info(ctx, "123")
		logger.End(ctx)

		request, _ := http.NewRequest("GET", "http://localhost:8080/test", nil)
		logger.HttpInject(ctx, request)
		client := &http.Client{}
		client.Do(request)
	}

	// wait
	<-make(chan bool)
}

func TestMaxspan(*testing.T) {
	conf := logger.Config{
		Debug:              true,
		EnableTrace:        true,
		TracerProviderType: "file",
	}

	logger.Init(conf, logger.Int64("ID", 1))
	ctx1 := logger.Start(context.Background(), "test1")
	defer logger.End(ctx1)
	logger.Warn(ctx1, "test info", logger.String("name1", "test"))
	logger.Error(ctx1, "test info", logger.Bool("sex1", true))
	logger.Error(ctx1, "test info", logger.Int("age1", 30))
	for i := 0; i < 300; i++ {
		ctx2 := logger.Start(ctx1, "test2", logger.Int("spanNum", 2))
		fmt.Println(logger.TraceID(ctx2))
		logger.Error(ctx2, "test2 info", logger.String("name2", "test"))
		logger.Info(ctx2, "test2 info", logger.Bool("sex2", true))
		logger.Info(ctx2, "test2 info", logger.Int("age2", 30))
		logger.End(ctx2)
	}
}
