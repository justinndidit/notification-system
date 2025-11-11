package logger

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/pkgerrors"
)

func NewLogger(serviceName string) zerolog.Logger {
	zerolog.TimeFieldFormat = "2006-01-02 15:04:05"
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack
	consoleWriter := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: "2006-01-02 15:04:05"}
	logger := zerolog.New(consoleWriter).
		Level(zerolog.WarnLevel).
		With().
		Timestamp().
		Str("service", serviceName).
		Logger()

	return logger
}

func NewPgxLogger() zerolog.Logger {
	writer := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: "2006-01-02 15:04:05",
		FormatFieldValue: func(i any) string {
			switch v := i.(type) {
			case string:
				// Clean and format SQL for better readability
				if len(v) > 200 {
					// Truncate very long SQL statements
					return v[:200] + "..."
				}
				return v
			case []byte:
				var obj interface{}
				if err := json.Unmarshal(v, &obj); err == nil {
					pretty, _ := json.MarshalIndent(obj, "", "    ")
					return "\n" + string(pretty)
				}
				return string(v)
			default:
				return fmt.Sprintf("%v", v)
			}
		},
	}

	return zerolog.New(writer).
		Level(zerolog.WarnLevel).
		With().
		Timestamp().
		Str("component", "database").
		Logger()
}
