package logger

import (
	"os"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// var zlogger *zap.Logger
type zapLogger struct {
	Logger    *zap.Logger
	lumLogger *lumberjack.Logger
}

var rotateCrondOnce sync.Once

// newZLogger init a zap logger
func newZapLogger(conf Config) zapLogger {
	// maxage default 7 days
	if config.MaxAge == 0 {
		config.MaxAge = 7
	}
	if conf.File == "" {
		conf.File = "./logs/run.log"
	}
	if conf.MaxBackups == 0 {
		conf.MaxBackups = 15
	}
	// log rolling config
	hook := lumberjack.Logger{
		Filename:   conf.File,
		MaxSize:    conf.MaxSize,
		MaxBackups: conf.MaxBackups,
		MaxAge:     conf.MaxAge,
		LocalTime:  true,
		Compress:   conf.Compress,
	}
	// Multi writer
	// lumberWriter and consoleWrite
	var multiWriter zapcore.WriteSyncer
	var writeSyncers []zapcore.WriteSyncer
	if conf.Output == "file" {
		writeSyncers = append(writeSyncers, zapcore.AddSync(&hook))
	} else {
		writeSyncers = append(writeSyncers, zapcore.AddSync(os.Stdout))
	}
	if len(writeSyncers) > 0 {
		multiWriter = zapcore.NewMultiWriteSyncer(writeSyncers...)
	}

	// encoderConfig
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:       "time",
		LevelKey:      "level",
		NameKey:       "logger",
		CallerKey:     "caller",
		FunctionKey:   zapcore.OmitKey,
		MessageKey:    "msg",
		StacktraceKey: "stacktrace",
		LineEnding:    zapcore.DefaultLineEnding,
		EncodeLevel:   zapcore.LowercaseLevelEncoder,
		EncodeTime: func(t time.Time, encoder zapcore.PrimitiveArrayEncoder) {
			encoder.AppendString(t.Format("2006-01-02 15:04:05"))
		},
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
	// logLevel
	// Encoder console or json
	enco := zapcore.NewJSONEncoder(encoderConfig)
	var atomicLevel zap.AtomicLevel
	if conf.Debug {
		atomicLevel = zap.NewAtomicLevelAt(zap.InfoLevel)
	} else {
		atomicLevel = zap.NewAtomicLevelAt(zap.ErrorLevel)
	}

	// new core config
	core := zapcore.NewCore(
		enco,
		multiWriter,
		atomicLevel,
	)

	// new logger
	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	return zapLogger{
		Logger:    logger,
		lumLogger: &hook,
	}
}

// rotateCrond
func (zl zapLogger) rotateCrond(conf Config) {
	if conf.Rotate != "" {
		rotateCrondOnce.Do(func() {
			cron := cron.New(cron.WithSeconds())
			cron.AddFunc(conf.Rotate, func() {
				zl.lumLogger.Rotate()
			})
			cron.Start()
		})
	}
}
