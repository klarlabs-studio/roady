package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	inframcp "github.com/felixgeelhaar/roady/internal/infrastructure/mcp"
	"github.com/felixgeelhaar/roady/internal/infrastructure/wiring"
	"github.com/felixgeelhaar/roady/pkg/domain/provenance"
	"github.com/spf13/cobra"
)

var (
	mcpTransport string
	mcpAddr      string
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the Roady MCP server",
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Getenv("ROADY_SKIP_MCP_START") == "true" {
			return nil
		}
		// Declare the surface before any service is built, so events this
		// process records are attributed to an agent rather than to a CLI run.
		wiring.SetProvenanceSurface(provenance.SurfaceMCP)

		cwd, err := getProjectRoot()
		if err != nil {
			return fmt.Errorf("resolve project path: %w", err)
		}
		server, err := inframcp.NewServer(cwd)
		if err != nil {
			return fmt.Errorf("create MCP server: %w", err)
		}

		transport := strings.ToLower(mcpTransport)

		// For stdio transport, redirect stderr to a log file (or /dev/null)
		// so that stray fmt.Println / panic tracebacks from dependencies
		// don't corrupt the JSON-RPC stream on stdout.
		if transport == "stdio" || transport == "" {
			if f, err := redirectStderr(); err == nil && f != nil {
				defer f.Close() //nolint:errcheck // best-effort close on stderr redirect
			}
		}

		// Cancel the server context on SIGINT/SIGTERM so in-flight
		// handlers can drain and connections close cleanly.
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sig
			cancel()
		}()

		switch transport {
		case "stdio", "":
			err = server.ServeStdio(ctx)
		case "http":
			err = server.ServeHTTP(ctx, mcpAddr)
		case "ws", "websocket":
			err = server.ServeWebSocket(ctx, mcpAddr)
		case "grpc":
			err = server.ServeGRPC(ctx, mcpAddr)
		default:
			err = fmt.Errorf("unsupported transport: %s", transport)
		}

		// context.Canceled is expected on signal-driven shutdown.
		if err != nil && ctx.Err() == context.Canceled {
			return nil
		}
		return err
	},
}

func init() {
	mcpCmd.Flags().StringVar(&mcpTransport, "transport", "stdio", "Transport to use (stdio, http, ws, grpc)")
	mcpCmd.Flags().StringVar(&mcpAddr, "addr", ":8080", "Address for http/ws/grpc transports")
	RootCmd.AddCommand(mcpCmd)
}
