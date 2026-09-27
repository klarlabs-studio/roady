package mcp

import (
	"context"
	"fmt"
)

// InitArgs and handleInit initialise a project for tests. roady_init is no
// longer an MCP tool (a person runs `roady init`), but tests still need a
// project to call the remaining tools against.
type InitArgs struct {
	Name        string
	ProjectPath string
	Project     string
}

func (s *Server) handleInit(_ context.Context, args InitArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return nil, err
	}
	if err := svc.Init.InitializeProject(args.Name); err != nil {
		return nil, err
	}
	return fmt.Sprintf("Project %s initialized successfully", args.Name), nil
}
