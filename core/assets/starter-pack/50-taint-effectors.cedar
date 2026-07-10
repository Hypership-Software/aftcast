// Taint effector (ADR-005): once a session has ingested untrusted content it is
// marked tainted, and external network calls are cut without approval — the
// lethal-trifecta break (private data + untrusted content + external comms).
@id("taint-blocks-external-fetch")
forbid (
    principal,
    action == Action::"net_fetch",
    resource
)
when { context.tainted };
