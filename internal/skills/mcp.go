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

func NewRegistry(rootPath string) *Registry {
	// Load existing tools from disk
	return &Registry{
		LibraryPath: filepath.Join(rootPath, "verified"),
		ConfigPath:  filepath.Join(rootPath, "mcp_config.json"),
		Tools:       make(map[string]ToolDef),
	}
}

// RegisterTool promotes a staging script to a permanent MCP tool.
func (r *Registry) RegisterTool(name, code, desc string) error {
	// 1. Save the source code to the "verified" folder
	filename := filepath.Join(r.LibraryPath, name, "main.go")
	if err := r.saveFile(filename, code); err != nil {
		return err
	}

	// 2. Build the binary (We need it executable for MCP)
	// (Implementation: Run "go build" via os/exec or the Sandbox)

	// 3. Define the MCP Tool Schema
	tool := ToolDef{
		Name:        name,
		Description: desc,
		InputSchema: generateJsonSchema(code), // Helper to reflectively gen schema
		Entrypoint:  filename,
	}

	// 4. Update Memory
	r.Tools[name] = tool
	return r.syncConfig()
}

// syncConfig writes the mcp.json file for external usage.
func (r *Registry) syncConfig() error {
	data, _ := json.MarshalIndent(r.Tools, "", "  ")
	return os.WriteFile(r.ConfigPath, data, 0644)
}

// ListTools returns the tools for Context Injection.
func (r *Registry) ListTools() []ToolDef {
	var list []ToolDef
	for _, t := range r.Tools {
		list = append(list, t)
	}
	return list
}
