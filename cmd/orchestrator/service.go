package main

import (
	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/flags-gg/orchestrator/internal/config"
	"github.com/flags-gg/orchestrator/internal/service"
)

var (
	BuildVersion = "0.0.1"
	BuildHash    = "unknown"
	ServiceName  = "service"
)

func main() {
	logs.Local().Infof("Starting %s version %s (build %s)", ServiceName, BuildVersion, BuildHash)

	cfg, err := config.Build()
	if err != nil {
		_ = logs.Local().Errorf("Failed to build config: %v", err)
		return
	}

	if err := service.New(cfg).Start(); err != nil {
		_ = logs.Local().Errorf("Failed to start service: %v", err)
		return
	}
}
