package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"{{ .Computed.module_name_final }}/internal/app"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("run server failed: " + rootCause(err).Error())
		os.Exit(1)
	}
}

func rootCause(err error) error {
	for {
		next := errors.Unwrap(err)
		if next == nil {
			return err
		}
		err = next
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("server", flag.ContinueOnError)
	confPath := flags.String("c", "conf", "configuration directory")
	if err := flags.Parse(args); err != nil {
		return err
	}

	ctx, stop := app.SignalContext()
	defer stop()

	application, err := app.New(ctx, *confPath)
	if err != nil {
		return fmt.Errorf("initialize application: %w", err)
	}
	defer application.Close()

	if err := application.Run(ctx); err != nil {
		return fmt.Errorf("run application: %w", err)
	}
	return nil
}
