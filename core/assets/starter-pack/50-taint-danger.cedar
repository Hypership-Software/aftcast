// Taint effector (ADR-005): once a session has ingested untrusted content it is
// marked tainted, and an external network call from that session is classified
// DANGER — the lethal-trifecta signal (private data + untrusted content +
// external comms). Atlas flags it; the org's controls decide whether to stop it.
@id("danger-tainted-external-fetch")
forbid (
    principal,
    action == Action::"net_fetch",
    resource
)
when { context.tainted };
