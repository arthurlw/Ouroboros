package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ToolDef represents a tool in the MCP format.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"` // JSON Schema for arguments
	Entrypoint  string          `json:"entrypoint"`   // Path to the executable/script
}

// Registry manages the agent's long-term memory.
type Registry struct {
	LibraryPath string            // e.g., "ouroboros/skills_library/verified"
	ConfigPath  string            // e.g., "ouroboros/mcp_config.json"
	Tools       map[string]ToolDef
}

// NewRegistry initializes the skill memory.
func NewRegistry(rootPath string) *Registry {
	// Ensure directories exist
	verifiedPath := filepath.Join(rootPath, "verified")
	os.MkdirAll(verifiedPath, 0755)

	// Load existing tools (if any)
	tools := make(map[string]ToolDef)
	configPath := filepath.Join(rootPath, "mcp_config.json")

	if data, err := os.ReadFile(configPath); err == nil {
		_ = json.Unmarshal(data, &tools)
	}

	return &Registry{
		LibraryPath: verifiedPath,
		ConfigPath:  configPath,
		Tools:       tools,
	}
}

// RegisterTool promotes a staging script to a permanent MCP tool.
func (r *Registry) RegisterTool(name, code, desc string) error {
	// 1. Create the specific tool directory
	toolDir := filepath.Join(r.LibraryPath, name)
	if err := os.MkdirAll(toolDir, 0755); err != nil {
		return fmt.Errorf("failed to create tool dir: %w", err)
	}

	// 2. Save the source code
	filename := filepath.Join(toolDir, "main.go")
	if err := r.saveFile(filename, code); err != nil {
		return err
	}

	// 3. Define the MCP Tool Schema
	// For V1, we generate a generic schema that accepts a string argument.
	tool := ToolDef{
		Name:        name,
		Description: desc,
		InputSchema: r.generateJsonSchema(),
		Entrypoint:  filename,
	}

	// 4. Update Memory
	r.Tools[name] = tool
	return r.syncConfig()
}

// ListTools returns the tools for Context Injection.
func (r *Registry) ListTools() []ToolDef {
	var list []ToolDef
	for _, t := range r.Tools {
		list = append(list, t)
	}
	return list
}

// GetActiveMCPTools is an alias for ListTools to match Interface requirements if needed
func (r *Registry) GetActiveMCPTools() []ToolDef {
	return r.ListTools()
}

// --- Helper Functions ---

func (r *Registry) saveFile(path string, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

func (r *Registry) syncConfig() error {
	data, _ := json.MarshalIndent(r.Tools, "", "  ")
	return os.WriteFile(r.ConfigPath, data, 0644)
}

func (r *Registry) generateJsonSchema() json.RawMessage {
	// A simple default schema for V1 tools
	schema := `
	{
		"type": "object",
		"properties": {
			"args": {
				"type": "string",
				"description": "Arguments for the tool"
			}
		}
	}`
	return json.RawMessage(schema)
}
