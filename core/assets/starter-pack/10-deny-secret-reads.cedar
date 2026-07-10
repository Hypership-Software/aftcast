// Deny agent access to credential and secret files. Deny-overrides means this
// wins over the broad file_read permit in 40-permit-safe.cedar.
@id("deny-secret-file-access")
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
