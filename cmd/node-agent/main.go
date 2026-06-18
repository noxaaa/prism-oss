package main

import (
	"context"
	"log"
	"os"

	"github.com/noxaaa/prism-oss/internal/agent"
	"github.com/noxaaa/prism-oss/internal/forward"
)

func main() {
	cfg, err := agent.LoadRuntimeConfigFromArgs(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}
	if cfg.ControlPlaneURL == "" {
		log.Fatal("CONTROL_PLANE_URL is required")
	}
	supervisor := forward.NewSupervisor()
	defer supervisor.Close()
	runtime := agent.NewNodeRuntime(cfg, supervisor)
	log.Printf("%s node-agent connecting to %s", cfg.AppName, cfg.ControlPlaneURL)
	if err := runtime.Run(context.Background()); err != nil {
		log.Fatal(err)
	}
}
