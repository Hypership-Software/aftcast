// Classify tampering with Aftcast itself as DANGER: the agent reading/writing
// the observer's own state directory, or trying to disable Aftcast. This
// surfaces "the agent tried to turn off its own monitoring" as a risk signal —
// it does not prevent it (non-removability is the managed tier's job, ADR-007).
// The legacy .gated path and binary name stay covered: pre-rename installs
// exist and their state deserves the same signal.
@id("danger-self-directory-access")
forbid (
    principal,
    action in [Action::"file_read", Action::"file_write"],
    resource
)
when {
    context has file &&
    (context.file like "*/.aftcast/*" || context.file like "*/.gated/*")
};

@id("danger-self-disable")
forbid (
    principal,
    action == Action::"exec",
    resource
)
when {
    (context.verbs.contains("aftcast") || context.verbs.contains("gated")) &&
    (context.argv.contains("off") || context.argv.contains("uninstall"))
};
