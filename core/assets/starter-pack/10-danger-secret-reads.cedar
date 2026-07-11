// Classify agent access to credential and secret files as DANGER. Modeled as a
// Cedar forbid so danger-overrides wins over the broad safe file_read rule in
// 40-safe-baseline.cedar; the engine maps a matching forbid to Risk=danger.
// Atlas records the classification — it does not block the read.
@id("danger-secret-file-access")
forbid (
    principal,
    action in [Action::"file_read", Action::"file_write"],
    resource
)
when {
    context has file &&
    (context.file like "*.env" ||
     context.file like "*.env.*" ||
     context.file like "*.pem" ||
     context.file like "*.key" ||
     context.file like "*/.ssh/*" ||
     context.file like "*/.aws/credentials*" ||
     context.file like "*/.aws/config*")
};
