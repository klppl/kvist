package build

import (
	"context"

	"github.com/klppl/kvist/internal/protocol"
)

// Placeholder is the builder used until the renderer exists (Phase 3): it
// accepts every revision and produces no output.
var Placeholder = BuilderFunc(func(ctx context.Context, site, revision, buildID string) ([]protocol.Warning, error) {
	return []protocol.Warning{{Code: protocol.WarnBuild, Message: "content stored; this server version does not render sites yet"}}, nil
})
