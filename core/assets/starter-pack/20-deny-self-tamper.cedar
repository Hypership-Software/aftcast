// Self-protection: the agent must not read/write the gate's own state directory
// or disable the gate. Same-user malware can still defeat the free tier (that is
// the managed-tier argument, ADR-007) — this stops the agent, not a determined
// local attacker.
@id("deny-self-directory-access")
forbid (
    principal,
    action in [Action::"file_read", Action::"file_write"],
    resource
)
when {
    context has file &&
    (context.file like "*/.gated/*" || context.file like "*/.atlas/*")
};

@id("deny-self-disable")
forbid (
    principal,
    action == Action::"exec",
    resource
)
when {
    context.verbs.contains("gated") &&
    (context.argv.contains("off") || context.argv.contains("uninstall"))
};
