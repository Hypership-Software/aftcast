// Baseline permits for low-risk operations. Everything not permitted here and
// not forbidden above resolves to `ask` (the safe default). Reads are permitted
// broadly because reading is non-destructive; the secret/self forbids above
// carve out the dangerous cases via deny-overrides. The "permit in-project
// writes/exec" rule is intentionally NOT here yet — it needs a computed
// in_project flag; until then those operations correctly fall through to `ask`.
@id("permit-file-read")
permit (
    principal,
    action == Action::"file_read",
    resource
);

@id("permit-net-search")
permit (
    principal,
    action == Action::"net_search",
    resource
);
