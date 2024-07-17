package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger struct {
	zap *zap.Logger
}

func NewLogger(level string) (*Logger, error) {
	// convert the text logging level to zap.AtomicLevel
	lvl, err := zap.ParseAtomicLevel(level)
	if err != nil {
		return nil, err
	}

	// create a new logger configuration
	config := zap.NewProductionConfig()
	// set the level
	config.Level = lvl
	// config.OutputPaths = []string{"stdout", "./logs/" + logFile}
	logger, err := config.Build(zap.AddCaller())
	if err != nil {
		return nil, err
	}

	return &Logger{zap: logger}, err
}

func (l Logger) Debug(msg string, fields ...zap.Field) {
	l.writer().Debug(msg, fields...)
}

func (l Logger) Info(msg string, fields ...zap.Field) {
	l.writer().Info(msg, fields...)
}

func (l Logger) Warn(msg string, fields ...zapcore.Field) {
	l.writer().Warn(msg, fields...)
}

func (l Logger) writer() *zap.Logger {
	noOpLogger := zap.NewNop()
	if l.zap == nil {
		return noOpLogger
	}

	return l.zap
}
