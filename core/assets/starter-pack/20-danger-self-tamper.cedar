// Classify tampering with Atlas itself as DANGER: the agent reading/writing the
// gate's own state directory, or trying to disable Atlas. This surfaces "the
// agent tried to turn off its own monitoring" as a risk signal — it does not
// prevent it (non-removability is the managed tier's job, ADR-007).
@id("danger-self-directory-access")
forbid (
    principal,
    action in [Action::"file_read", Action::"file_write"],
    resource
)
when {
    context has file &&
    (context.file like "*/.gated/*" || context.file like "*/.atlas/*")
};

@id("danger-self-disable")
forbid (
    principal,
    action == Action::"exec",
    resource
)
when {
    context.verbs.contains("gated") &&
    (context.argv.contains("off") || context.argv.contains("uninstall"))
};
