# Project Ouroboros

**A compiler-verified, LLM-based code synthesis system with persistent tool reuse.**

> **Version:** 1.0 (Alpha)
> **Language:** Go (Golang)
> **Infrastructure:** Dockerized Sandbox, LLM Integration (Groq/Llama 3.3)

---

## 1. Summary

Ouroboros is a goal-driven execution harness for LLMs. Unlike standard chatbots which generate static text, Ouroboros functions as a rigorous build system that generates **executable capabilities**.

When given a goal, it plans a software solution, writes the source code, compiles it, tests it within a secure environment, and iterates until the code compiles and passes verification.

Unlike ephemeral agents, Ouroboros can use its created tools to answer prompts and implements **persistent tool reuse**. Once it builds a functioning tool, it serializes it to a local registry (`skills_library`). This allows the system to rehydrate context and refactor existing logic in future tasks without rebuilding tools from scratch.

---

## 2. System Architecture

Ouroboros operates on a cyclical **"Plan-Build-Test-Execute"** loop. It is architected as a compiled Go binary that orchestrates an LLM inference engine and a Docker execution environment.

### Core Components

* **The Planner:** Analyzes the user's high-level goal and breaks it down into atomic steps. It queries the `skills_library` first—if a tool exists, it assigns a "Refactor" task; if not, it assigns a "Create" task.
* **The Registry:** A file-system-based storage mechanism where verified tools reside. This serves as a regression guard; before generating code, the system checks this registry to ensure optimized logic is not overwritten by boilerplate.
* **The Generator:** Produces Go source code (`main.go`) and test suites (`main_test.go`). It is context-aware, distinguishing between "New Tool" mode and "Refactor" mode.
* **The Sandbox:** An ephemeral Docker Container that mounts the code, compiles it, and runs tests. This allows Ouroboros to execute operations like port scanning or file I/O without risking the host machine.
* **The Critic:** Parses raw compiler logs and test output. If a tool fails, it generates specific, structured feedback (e.g., "Test failed on line 42: Index out of range") to guide the next generation cycle.

### The Convergence Loop
The system solves the issue of LLM hallucinations by trapping the model in a feedback loop until verification succeeds:
1.  **Ping-Pong Strategy:** Alternates focus between fixing `main.go` (Implementation) and `main_test.go` (Test Suite) to prevent logical deadlocks.
2.  **Smart Sanitization:** Uses Regex-based filtering to strip duplicate functions and prevent "Redeclaration Errors" common in LLM outputs.

### Asymmetric Visibility

The two refinement roles do not see the same thing. An agent that can read both files will make them agree with each other rather than with the specification: it edits the test until the current implementation passes. The result is a tool that is green and wrong.

The separation is enforced by what is put into the prompt, not by instructing the model to ignore what it can see.

* **The Implementer (`refineImplementation`)** receives `main.go`, the original step specification, and the Critic's failure feedback — failing test names, expected versus actual values, panic messages and line numbers. It does not receive the source of `main_test.go`.
* **The Test Author (`refineTestSuite`)** receives `main_test.go`, the original step specification, the same failure feedback, and a signature-only view of `main.go`. It does not receive any function body.

The signature view is produced by `ExtractSignatures` (`internal/agent/signatures.go`), which parses the file with `go/parser` and prints it back with every function body removed, keeping the package clause, imports, type and struct declarations, constants and function signatures. Unlike the regex sanitizers, this is a real parse rather than a best-effort match. If the generated source does not parse — which happens while it is mid-repair — the view degrades to a `// [implementation unparseable this iteration]` placeholder; it never falls back to the full source, since that would hand over exactly what the split exists to withhold.

The test author needs the interface in order to write a suite that compiles, so full blindness is not available. Signature-only is the compromise. Compiler errors that quote lines of `main.go` are still passed through verbatim in both directions: an error message is feedback, not source access.

---

## Phase 1

Phase 1 focused on the implementation of the **Tool Builder Engine**.

We have successfully established a stable feedback loop capable of self-generating, compiling, and verifying Go code. The system can now autonomously handle the lifecycle of tool creation—from receiving a prompt to persisting a compiled binary—without human intervention in the debugging loop, as well as use its tools to answer prompts.

---

## Phase 2

The objective of Phase 2 is to enable **Surgical Debugging**. The system must be able to read, debug, and patch its own core source code without needing to ingest the entire repository context.

### The Challenge: Context Management
Feeding the entire Ouroboros source code into the LLM context window is inefficient and leads to "context explosion," degrading the model's reasoning capabilities.

### The Solution: Introspection Tools
To solve this, the Ouroboros engine was tasked with creating its own debugging utilities. By mounting the Docker container on the host's source code, the system built a two-step mechanism to facilitate "self-reading" without token bloat:

**1. The Skeleton Tool (`map_project`)**
* **Function:** Generates a lightweight directory tree of the project (Repo Root + Short Summary of each file).
* **Utility:** Allows the Planner to visualize the project architecture and locate relevant logic files without reading their contents.

**2. The Reader Tool (`read_file`)**
* **Function:** Targeted file reading (similar to `claude code`).
* **Utility:** Allows the Planner to read specific files on demand.

**The Introspection Workflow:**
1.  **Ingest:** The container mounts the project source.
2.  **Map:** The system runs `map_project` to understand the file structure.
3.  **Correlate:** It matches runtime logs to specific files in the map.
4.  **Read:** It runs `read_file` to ingest *only* the specific file identified as buggy.
5.  **Patch:** It generates a fix based on this high-precision, low-noise context.

