package mcp

import (
	"context"
	"encoding/json"

	mcplib "go.klarlabs.de/mcp"
)

// SchemaVersion is the current MCP tool schema version (semver).
const SchemaVersion = "3.1.0"

// DeprecatedField records a field or tool that has been deprecated.
type DeprecatedField struct {
	Tool      string `json:"tool"`
	Field     string `json:"field"`
	Since     string `json:"since"`
	RemovedIn string `json:"removed_in"`
	Migration string `json:"migration"`
}

// deprecatedFields returns the list of currently deprecated fields.
func deprecatedFields() []DeprecatedField {
	return []DeprecatedField{}
}

type schemaResponse struct {
	SchemaVersion string            `json:"schema_version"`
	ServerVersion string            `json:"server_version"`
	Deprecated    []DeprecatedField `json:"deprecated"`
	Changelog     string            `json:"changelog"`
}

func (s *Server) registerSchemaResource() {
	s.mcpServer.Resource("roady://schema").
		Name("roady://schema").
		Description("MCP tool schema version and deprecation info").
		MimeType("application/json").
		Handler(func(_ context.Context, _ string, _ map[string]string) (*mcplib.ResourceContent, error) {
			resp := schemaResponse{
				SchemaVersion: SchemaVersion,
				ServerVersion: Version,
				Deprecated:    deprecatedFields(),
				Changelog:     "https://github.com/felixgeelhaar/roady/blob/main/docs/mcp-schema-changelog.md",
			}
			data, err := json.Marshal(resp)
			if err != nil {
				return nil, err
			}
			return &mcplib.ResourceContent{
				URI:      "roady://schema",
				MimeType: "application/json",
				Text:     string(data),
			}, nil
		})
}
