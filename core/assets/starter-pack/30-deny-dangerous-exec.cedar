// Dangerous shell operations. BEST-EFFORT (ADR-006): these match the tokenized
// argv/verbs only, so deliberately obfuscated shell can evade them. Faithful
// coverage arrives with the real adapter payloads (Task 10). Precise
// "outside the project root" detection needs a computed in_project flag the
// daemon will supply later; for now recursive-force removal is denied broadly.
@id("deny-rm-rf")
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

@id("deny-force-push")
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
