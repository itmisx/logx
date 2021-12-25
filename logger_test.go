package logger

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/itmisx/logger/propagation/extract"
)

func TestTrace(*testing.T) {
	conf := Config{
		Debug:              true,
		EnableTrace:        true,
		EnableLog:          true,
		File:               "./run.log",
		TracerProviderType: "file",
		TraceSampleRatio:   1,
		JaegerServer:       "http://127.0.0.1:14268/api/traces",
	}

	Init(conf, Int64("ID", 1))
	ctx1 := Start(context.Background(), "test1")
	Warn(ctx1, "test info", String("name1", "test"))
	Error(ctx1, "test info", Bool("sex1", true))
	Error(ctx1, "test info", Int("age1", 30))
	End(ctx1)

	ctx2 := Start(ctx1, "test2", Int("spanNum", 2))
	Error(ctx2, "test2 info", String("name2", "test"))
	Info(ctx2, "test2 info", Bool("sex2", true))
	Info(ctx2, "test2 info", Int("age2", 30))
	End(ctx2)
}

func TestCustomizeTraceID(*testing.T) {
	conf := Config{
		Debug:              true,
		EnableTrace:        true,
		EnableLog:          true,
		File:               "./run.log",
		TracerProviderType: "file",
		TraceSampleRatio:   1,
		JaegerServer:       "http://127.0.0.1:14268/api/traces",
	}
	Init(conf, Int64("ID", 1))
	traceID := GenTraceID()
	spanID := GenSpanID()
	ctx, err := NewRootContext(traceID, spanID)
	if err != nil {
		fmt.Println(err)
	}
	ctx = Start(ctx, "test1")
	Warn(ctx, "test info", String("name1", "test"))
	Error(ctx, "test info", Bool("sex1", true))
	Error(ctx, "test info", Int("age1", 30))
	End(ctx)

	<-make(chan bool)
}

func TestLog(*testing.T) {
	conf := Config{
		Debug:       true,
		EnableLog:   true,
		EnableTrace: true,
		Rotate:      "*/5 * * * * *",
		File:        "./run.log",
	}
	ctx := context.Background()
	Init(conf)
	Info(ctx, "log info")
	Warn(ctx, "log warn")
	Error(ctx, "log error")
}

func TestPropagationWithGlobalPropagators(t *testing.T) {
	// logger init
	{
		conf := Config{
			Debug:       true,
			EnableTrace: true,
			// TracerProviderType: "file",
			TraceSampleRatio: 1,
			JaegerServer:     "http://127.0.0.1:14268/api/traces",
		}
		Init(conf, Int64("ID", 1))
	}

	// new gin server
	{
		router := gin.Default()
		router.Use(extract.GinMiddleware(""))
		router.GET("/user/:id", func(c *gin.Context) {
			ctx := Start(c.Request.Context(), "123")
			Info(ctx, "456")
			End(ctx)
		})
		router.POST("/open-api/control/signal-test", func(c *gin.Context) {
			ctx := Start(c.Request.Context(), "123")
			Info(ctx, "789")
			End(ctx)
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
		ctx := Start(context.Background(), "123")
		Info(ctx, "123")
		End(ctx)

		request, _ := http.NewRequest("GET", "http://localhost:8080/test", nil)
		HttpInject(ctx, request)
		client := &http.Client{}
		client.Do(request)
	}

	// wait
	<-make(chan bool)
}
