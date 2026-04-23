package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/arthurlw/ouroboros/internal/agent"
	"github.com/arthurlw/ouroboros/internal/sandbox"
	"github.com/arthurlw/ouroboros/internal/skills"
	"github.com/joho/godotenv"
)

func main() {
	// 0. Load .env file if present (silently ignore if missing)
	_ = godotenv.Load()

	// 1. CLI Parsing
	goalPtr := flag.String("goal", "", "The objective for Ouroboros")
	flag.Parse()

	if *goalPtr == "" {
		slog.Error("Please provide a goal using -goal 'Your Goal'")
		os.Exit(1)
	}

	// 2. Setup Logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// 3. Infrastructure Initialization
	cwd, _ := os.Getwd()
	libraryPath := filepath.Join(cwd, "skills_library")

	// Modules
	registry := skills.NewRegistry(libraryPath)

	// Docker connection
	gym, err := sandbox.NewSandbox("ouroboros-executor:latest", filepath.Join(libraryPath, "staging"))
	if err != nil {
		slog.Error("Failed to connect to Docker", "error", err)
		os.Exit(1)
	}

	// AI Modules
	llmClient, err := agent.NewClient()
	if err != nil {
		slog.Error("Failed to initialize LLM provider", "error", err)
		os.Exit(1)
	}
	planner := agent.NewPlanner(llmClient)
	critic := agent.NewCritic()
	generator := agent.NewGenerator(llmClient)

	// 4. Assemble Agent
	ouroboros := &agent.Agent{
		Planner:   planner,
		Critic:    critic,
		Sandbox:   gym,
		Registry:  registry,
		Generator: generator,
		LLM:       llmClient,
		Context:   context.Background(),
	}

	// 5. Start
	slog.Info("🐍 OUROBOROS STARTUP COMPLETE", "goal", *goalPtr)
	if err := ouroboros.EvolutionLoop(context.Background(), *goalPtr); err != nil {
		slog.Error("Evolution Failed", "error", err)
		os.Exit(1)
	}

	slog.Info("🏁 GOAL ACCOMPLISHED")
}
