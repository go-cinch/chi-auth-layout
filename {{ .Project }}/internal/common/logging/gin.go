package logging

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// Gin exposes process-wide logging hooks. Install them once before creating
// engines; callbacks resolve the current slog logger so reinitialization works.
var ginLoggingOnce sync.Once

func initGinLogging() {
	ginLoggingOnce.Do(func() {
		gin.DebugPrintFunc = logGinDebug
		gin.DebugPrintRouteFunc = logGinRoute
		gin.DefaultErrorWriter = ginErrorWriter{}
	})
}

func logGinDebug(format string, values ...any) {
	message := strings.TrimSpace(fmt.Sprintf(format, values...))
	if warning, ok := strings.CutPrefix(message, "[WARNING]"); ok {
		warning = strings.TrimSpace(warning)
		if strings.HasPrefix(warning, `Running in "debug" mode.`) {
			warning = "debug mode; use GIN_MODE=release in production"
		}
		slog.Warn("gin: " + warning)
		return
	}
	slog.Debug("gin: " + message)
}

func logGinRoute(method, path, handler string, handlers int) {
	slog.Info("register route: "+method+" "+path,
		"handler", handler, "handlers", handlers)
}

type ginErrorWriter struct{}

func (ginErrorWriter) Write(data []byte) (int, error) {
	message := strings.TrimSpace(string(data))
	message = strings.TrimSpace(strings.TrimPrefix(message, "[GIN-debug] [ERROR]"))
	if message != "" {
		slog.Error("gin: " + message)
	}
	return len(data), nil
}
