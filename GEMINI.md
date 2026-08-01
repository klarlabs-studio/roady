# Roady Project Context

## Project Overview

**Roady** is a planning-first system of record for software work. It acts as a durable memory layer between intent (what we want to build), plans (how we will build it), and execution (the actual work). It is designed to help individuals, teams, and AI agents answer "What are we building?", "Why?", and "What's next?" without re-deriving context.

*   **Type:** CLI Tool & MCP Server (Local-first, Open Source Core)
*   **Goal:** To be the deterministic source of truth for software planning, preventing drift between specs and reality.
*   **Key Principles:** Planning before execution, explicit intent, drift detection, and local-first data ownership.

## Documentation Structure

The project is in the **Implementation / Alpha** phase. The `docs/` directory contains the foundational documents:

*   **`docs/vision.md`**: The high-level philosophy and "North Star".
*   **`docs/prd.md` (Product Requirements Document)**: Detailed capabilities for Roady Core.
*   **`ROADMAP.md`**: The public roadmap.
*   **`docs/tdd.md` (Technical Design Document)**: Architectural overview.

## Architecture & Design

Roady follows a **Domain-Driven Design (DDD)** approach with a clean separation of concerns:

*   **`pkg/domain/`**: Pure domain logic and entities (Spec, Planning, Drift, Policy).
*   **`pkg/application/`**: Use-case services that orchestrate domain logic.
*   **`internal/infrastructure/`**: Adapters for CLI (`cobra`), AI Providers, MCP, and Storage.
*   **`pkg/plugin/`**: Infrastructure for the plugin system (based on `hashicorp/go-plugin`).

## Project Status

**Current Phase:** Implementation / Alpha.
**Codebase:**
*   **CLI:** Fully scaffolded `cmd/roady` with commands for `init`, `spec`, `plan`, `drift`, `status`.
*   **Core Logic:** Implemented services in `pkg/application` for Specs, Plans, and Drift.
*   **AI:** Flexible provider architecture (`pkg/ai`) supporting Anthropic, Gemini, OpenAI, and Ollama.
*   **Plugins:** Architecture defined in `pkg/plugin` with stubbed implementations for GitHub, Jira, and Linear.

## Intended Usage

Roady is used via the command line or MCP:

```bash
# Workflow
roady init              # Initialize project
roady spec generate     # Create spec from context
roady plan generate     # Generate plan DAG
roady drift detect      # Compare plan vs reality
```