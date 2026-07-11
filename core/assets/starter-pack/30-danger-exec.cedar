// Classify dangerous shell operations as DANGER. BEST-EFFORT (ADR-006): these
// match the tokenized argv/verbs only, so deliberately obfuscated shell can
// evade them — acceptable for a risk signal, because Atlas observes rather than
// blocks, so a miss is an unflagged event, never an unsafe allow.
@id("danger-rm-rf")
forbid (
    principal,
    action == Action::"exec",
    resource
)
when {
    context.verbs.contains("rm") &&
    (context.argv.contains("-rf") ||
     context.argv.contains("-fr") ||
     context.argv.contains("-r") ||
     context.argv.contains("--recursive"))
};

@id("danger-force-push")
forbid (
    principal,
    action == Action::"exec",
    resource
)
when {
    context.verbs.contains("git") &&
    context.argv.contains("push") &&
    (context.argv.contains("--force") ||
     context.argv.contains("-f") ||
     context.argv.contains("--force-with-lease"))
};
