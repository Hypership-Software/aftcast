// Baseline SAFE classifications for low-risk operations. Modeled as Cedar
// permits (the engine maps a matching permit to Risk=safe). Anything neither
// classified safe here nor danger above is Risk=unknown. Reads are safe broadly
// because reading is non-destructive; the danger rules above carve out the
// sensitive cases via danger-overrides.
@id("safe-file-read")
permit (
    principal,
    action == Action::"file_read",
    resource
);

@id("safe-net-search")
permit (
    principal,
    action == Action::"net_search",
    resource
);
