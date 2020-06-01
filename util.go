package logjam

import (
	"context"
	"fmt"
	"log"
)

// LogLevel (modeled after Ruby log levels).
type LogLevel int

const (
	DEBUG LogLevel = 0
	INFO  LogLevel = 1
	WARN  LogLevel = 2
	ERROR LogLevel = 3
	FATAL LogLevel = 4
)

// Logger extends the standard log.Logger with methods that send log lines to both logjam
// and the embedded Logger. Even though all standard logger methods are available, you
// will want to use Fatal, Fatalf and Fatalln only during program startup/shutdown, as
// they abort the runnning process. Note that lines which are logged at a level below the
// configured log level will not be sent to the embedded logger, only forwarded to the
// logjam logger. Usually one would configure log level DEBUG for development and ERROR or
// even FATAL for production environments, as logs are sent to logjam and/or Graylog
// anyway.
type Logger struct {
	*log.Logger          // The embedded log.Logger.
	LogLevel    LogLevel // Log attemtps with a log level lower than this field are not forwarded to the embbeded logger.
}

func (l *Logger) logf(ctx context.Context, severity LogLevel, format string, args ...interface{}) {
	line := fmt.Sprintf(format, args...)
	if request := GetRequest(ctx); request != nil {
		request.Log(severity, line)
	}
	if severity >= l.LogLevel {
		l.Output(3, line)
	}
}

func (l *Logger) log(ctx context.Context, severity LogLevel, args ...interface{}) {
	line := fmt.Sprint(args...)
	if request := GetRequest(ctx); request != nil {
		request.Log(severity, line)
	}
	if severity >= l.LogLevel {
		l.Output(3, line)
	}
}

// Debugf logs with DEBUG severity.
func (l *Logger) Debugf(ctx context.Context, format string, args ...interface{}) {
	l.logf(ctx, DEBUG, format, args...)
}

// Debug logs with DEBUG severity.
func (l *Logger) Debug(ctx context.Context, args ...interface{}) {
	l.log(ctx, DEBUG, args...)
}

// Infof logs with INFO severity.
func (l *Logger) Infof(ctx context.Context, format string, args ...interface{}) {
	l.logf(ctx, INFO, format, args...)
}

// Info logs with INFO severity.
func (l *Logger) Info(ctx context.Context, args ...interface{}) {
	l.log(ctx, INFO, args...)
}

// Warnf logs WARN severity.
func (l *Logger) Warnf(ctx context.Context, format string, args ...interface{}) {
	l.logf(ctx, WARN, format, args...)
}

// Warn logs with WARN severity.
func (l *Logger) Warn(ctx context.Context, args ...interface{}) {
	l.log(ctx, WARN, args...)
}

// Errorf logs with ERROR severity.
func (l *Logger) Errorf(ctx context.Context, format string, args ...interface{}) {
	l.logf(ctx, ERROR, format, args...)
}

// Error logs with ERROR severity.
func (l *Logger) Error(ctx context.Context, args ...interface{}) {
	l.log(ctx, ERROR, args...)
}
