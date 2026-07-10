// Package policy maps enforcement descriptors onto Cedar authorization requests
// and resolves the three-valued verdict (deny/allow/ask) over a compiled Cedar
// policy set.
package policy

import (
	"github.com/Hypership-Software/atlas/internal/schema"
	"github.com/cedar-policy/cedar-go"
	"github.com/cedar-policy/cedar-go/types"
)

const (
	typeSession types.EntityType = "Session"
	typeAction  types.EntityType = "Action"
	typeFile    types.EntityType = "File"
	typeDomain  types.EntityType = "Domain"
	typeCommand types.EntityType = "Command"
	typeNone    types.EntityType = "None"
)

// ToCedar maps a Descriptor onto a Cedar request plus the entity store the
// evaluator needs. Pure: the same descriptor always yields the same request and
// entities. Principal = Session (carrying tainted/org as attributes so policies
// can gate on `principal.tainted`); action = the tool class; resource depends on
// the class; context carries the full descriptor detail for `when` clauses.
func ToCedar(d schema.Descriptor) (cedar.Request, types.EntityMap) {
	principal := cedar.NewEntityUID(typeSession, cedar.String(d.SessionID))

	entities := types.EntityMap{
		principal: types.Entity{
			UID: principal,
			Attributes: cedar.NewRecord(cedar.RecordMap{
				"tainted": types.Boolean(d.Tainted),
				"org":     cedar.String(d.Org),
			}),
		},
	}

	req := cedar.Request{
		Principal: principal,
		Action:    cedar.NewEntityUID(typeAction, cedar.String(string(d.ToolClass))),
		Resource:  resourceUID(d),
		Context:   buildContext(d),
	}
	return req, entities
}

// resourceUID derives the Cedar resource from the tool class. Cedar requires a
// resource on every request, so classes without a natural one get a None
// sentinel rather than an empty UID.
func resourceUID(d schema.Descriptor) types.EntityUID {
	switch d.ToolClass {
	case schema.ClassFileRead, schema.ClassFileWrite:
		if len(d.Files) > 0 {
			return cedar.NewEntityUID(typeFile, cedar.String(d.Files[0]))
		}
	case schema.ClassNetFetch, schema.ClassNetSearch:
		if d.Domain != "" {
			return cedar.NewEntityUID(typeDomain, cedar.String(d.Domain))
		}
	case schema.ClassExec:
		if len(d.Verbs) > 0 {
			return cedar.NewEntityUID(typeCommand, cedar.String(d.Verbs[0]))
		}
	}
	return cedar.NewEntityUID(typeNone, "none")
}

func buildContext(d schema.Descriptor) types.Record {
	rec := cedar.RecordMap{"tainted": types.Boolean(d.Tainted)}
	if len(d.Argv) > 0 {
		rec["argv"] = stringSet(d.Argv)
	}
	if len(d.Files) > 0 {
		rec["files"] = stringSet(d.Files)
	}
	if len(d.Verbs) > 0 {
		rec["verbs"] = stringSet(d.Verbs)
	}
	if d.Domain != "" {
		rec["domain"] = cedar.String(d.Domain)
	}
	if d.Cwd != "" {
		rec["cwd"] = cedar.String(d.Cwd)
	}
	if d.ProjectRoot != "" {
		rec["project_root"] = cedar.String(d.ProjectRoot)
	}
	if d.MCPServer != "" {
		rec["mcp_server"] = cedar.String(d.MCPServer)
	}
	if d.MCPTool != "" {
		rec["mcp_tool"] = cedar.String(d.MCPTool)
	}
	return cedar.NewRecord(rec)
}

// stringSet builds a Cedar Set from a string slice. Cedar sets are unordered and
// de-duplicated, which is the right model for policy membership checks
// (`context.argv.contains("--force")`); the verbatim argv is preserved in the
// TelemetryEvent for audit.
func stringSet(ss []string) types.Set {
	vals := make([]types.Value, len(ss))
	for i, s := range ss {
		vals[i] = cedar.String(s)
	}
	return cedar.NewSet(vals...)
}
