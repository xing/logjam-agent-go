package log

import (
	"context"
	"fmt"
	"log"

	"github.com/xing/logjam-agent-go"
)

// Logger extends the standard log.Logger with methods that send log lines to both logjam
// and the embedded Logger. Even though all standard logger methods are available, you
// will want use Fatal, Fatalf and Fatalln only during program startup/shutdown, as they
// abort the runnning process. To create a log line with log level FATAL use
// FatalPanic/FatalPanicf. Note that lines which are logged at a level below the
// configured log level will not be sent to the embedded logger, only forwarded to the
// logjam logger. Usually one would configure log level DEBUG for development and ERROR or
// even FATAL for production environments, as logs are sent to logjam and/or Graylog
// anyway.
type Logger struct {
	*log.Logger                 // The embedded log.Logger.
	LogLevel    logjam.LogLevel // Log attemtps with a log level lower than this field are not forwarded to the embbeded logger.
}

// New creates a new Logger
func New(logger *log.Logger, level logjam.LogLevel) *Logger {
	return &Logger{Logger: logger, LogLevel: level}
}

func (l *Logger) logf(ctx context.Context, severity logjam.LogLevel, format string, args ...interface{}) {
	line := fmt.Sprintf(format, args...)
	if request := logjam.GetRequest(ctx); request != nil {
		request.Log(severity, line)
	}
	if severity >= l.LogLevel {
		l.Logger.Output(3, line)
	}
}

func (l *Logger) log(ctx context.Context, severity logjam.LogLevel, args ...interface{}) {
	line := fmt.Sprint(args...)
	if request := logjam.GetRequest(ctx); request != nil {
		request.Log(severity, line)
	}
	if severity >= l.LogLevel {
		l.Logger.Output(3, line)
	}
}

// Debugf logs with DEBUG severity.
func (l *Logger) Debugf(ctx context.Context, format string, args ...interface{}) {
	l.logf(ctx, logjam.DEBUG, format, args...)
}

// Debug logs with DEBUG severity.
func (l *Logger) Debug(ctx context.Context, args ...interface{}) {
	l.log(ctx, logjam.DEBUG, args...)
}

// Infof logs with INFO severity.
func (l *Logger) Infof(ctx context.Context, format string, args ...interface{}) {
	l.logf(ctx, logjam.INFO, format, args...)
}

// Info logs with INFO severity.
func (l *Logger) Info(ctx context.Context, args ...interface{}) {
	l.log(ctx, logjam.INFO, args...)
}

// Warnf logs WARN severity.
func (l *Logger) Warnf(ctx context.Context, format string, args ...interface{}) {
	l.logf(ctx, logjam.WARN, format, args...)
}

// Warn logs with WARN severity.
func (l *Logger) Warn(ctx context.Context, args ...interface{}) {
	l.log(ctx, logjam.WARN, args...)
}

// Errorf logs with ERROR severity.
func (l *Logger) Errorf(ctx context.Context, format string, args ...interface{}) {
	l.logf(ctx, logjam.ERROR, format, args...)
}

// Error logs with ERROR severity.
func (l *Logger) Error(ctx context.Context, args ...interface{}) {
	l.log(ctx, logjam.ERROR, args...)
}

// FatalPanicf logs with FATAL severity, then panics. Please note that the standard Logger
// method exits the program, which is usually not a desired outcome in a long running
// server application.
func (l *Logger) FatalPanicf(ctx context.Context, format string, args ...interface{}) {
	l.logf(ctx, logjam.FATAL, format, args...)
	panic(fmt.Sprintf(format, args...))
}

// FatalPanic logs with FATAL severity, then panics. Please note that the standard Logger
// method exits the program, which is usually not a desired outcome in a long running
// server application.
func (l *Logger) FatalPanic(ctx context.Context, args ...interface{}) {
	l.log(ctx, logjam.FATAL, args...)
	panic(fmt.Sprint(args...))
}
