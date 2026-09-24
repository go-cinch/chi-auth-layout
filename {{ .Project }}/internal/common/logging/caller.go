package logging

import (
	"log/slog"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// callerAttribute formats slog's recorded call site, not the handler's own
// stack frame. The package location provides a project prefix for both normal
// builds (absolute paths) and -trimpath builds (module paths).
func callerAttribute() func([]string, slog.Attr) slog.Attr {
	_, file, _, ok := runtime.Caller(0)
	prefix := ""
	if ok {
		// This file lives in internal/common/logging beneath the project root.
		prefix = path.Dir(path.Dir(path.Dir(path.Dir(filepath.ToSlash(file))))) + "/"
	}
	return func(groups []string, attr slog.Attr) slog.Attr {
		if len(groups) != 0 || attr.Key != slog.SourceKey {
			return attr
		}
		source, ok := attr.Value.Any().(*slog.Source)
		if !ok {
			return attr
		}
		if source.File == "" || source.Line <= 0 {
			return slog.Attr{}
		}
		filename := strings.TrimPrefix(filepath.ToSlash(source.File), prefix)
		return slog.String("caller", filename+":"+strconv.Itoa(source.Line))
	}
}
