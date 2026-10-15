# Optional at first deployment. Configure a persistent value before offering TOTP
# enrollment. This is the ARN only; the key must never enter Terraform state.
variable "auth_encryption_secret_arn" {
  type        = string
  default     = ""
  description = "API-only Secrets Manager secret containing the base64 32-byte TOTP encryption key. Retain with database backups."
  validation {
    condition     = var.auth_encryption_secret_arn == "" || can(regex("^arn:aws:secretsmanager:", var.auth_encryption_secret_arn))
    error_message = "Supply a Secrets Manager ARN, never the plaintext AUTH_ENCRYPTION_KEY."
  }
}
