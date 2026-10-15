variable "region" {
  type    = string
  default = "ap-southeast-1"
}
variable "environment" {
  type    = string
  default = "staging"
}
variable "vpc_cidr" {
  type    = string
  default = "10.42.0.0/16"
}
variable "image_tag" {
  type    = string
  default = "latest"
}
variable "public_origin" {
  type = string
}
variable "alb_certificate_arn" {
  type = string
}
variable "edge_certificate_arn" {
  type = string
}
variable "web_domain" {
  type = string
}
variable "media_domain" {
  type = string
}
variable "alb_origin_domain" {
  type = string
}
variable "cloudfront_public_key_pem" {
  type = string
}
variable "secret_arns" {
  type = map(string)
  validation {
    condition     = !contains(keys(var.secret_arns), "MIGRATION_DATABASE_URL") && !contains(keys(var.secret_arns), "RUNTIME_DATABASE_PASSWORD") && !contains(keys(var.secret_arns), "AUTH_ENCRYPTION_KEY") && !contains(keys(var.secret_arns), "CRYPTORAILS_API_KEY") && !contains(keys(var.secret_arns), "CRYPTORAILS_WEBHOOK_SECRET") && alltrue([for key in ["DATABASE_URL", "REDIS_URL", "CLOUDFRONT_PRIVATE_KEY", "OPENROUTER_API_KEY"] : contains(keys(var.secret_arns), key)]) && alltrue([for arn in values(var.secret_arns) : can(regex("^arn:aws:secretsmanager:", arn))])
    error_message = "Supply required runtime Secrets Manager ARNs, never plaintext values."
  }
}
variable "redis_auth_secret_arn" {
  type = string
}
variable "budget_email" {
  type = string
}
variable "monthly_budget_usd" {
  type    = number
  default = 500
}
variable "desired_count" {
  type    = number
  default = 1
}
variable "worker_max_count" {
  type    = number
  default = 2
}
variable "multi_az" {
  type    = bool
  default = false
}
# secret_arns requires DATABASE_URL, REDIS_URL, CLOUDFRONT_PRIVATE_KEY and OPENROUTER_API_KEY.
# Historical Stripe keys are optional; new checkout uses cryptorails_secret_arns.
# redis_auth_secret_arn stores the raw Redis AUTH token, NOT REDIS_URL.

variable "provider_daily_budget_micros" {
  type    = number
  default = 0
  validation {
    condition     = var.provider_daily_budget_micros >= 0 && floor(var.provider_daily_budget_micros) == var.provider_daily_budget_micros
    error_message = "Provider budget must be a non-negative integer in USD micro-units."
  }
}
variable "provider_max_cost_micros" {
  type    = number
  default = 0
  validation {
    condition     = var.provider_max_cost_micros >= 0 && floor(var.provider_max_cost_micros) == var.provider_max_cost_micros
    error_message = "Maximum provider attempt cost must be a non-negative integer in USD micro-units."
  }
}
variable "openrouter_video_model" {
  type    = string
  default = ""
}
variable "ses_from" {
  type    = string
  default = ""
}
variable "ses_identity" {
  type    = string
  default = ""
}

variable "openrouter_story_model" {
  type    = string
  default = ""
}
variable "story_credits" {
  type    = number
  default = 0
  validation {
    condition     = var.story_credits >= 0 && floor(var.story_credits) == var.story_credits
    error_message = "Story credits must be a non-negative integer; zero disables paid story generation."
  }
}
variable "story_max_cost_micros" {
  type    = number
  default = 0
  validation {
    condition     = var.story_max_cost_micros >= 0 && floor(var.story_max_cost_micros) == var.story_max_cost_micros
    error_message = "Story attempt cost must be non-negative integer USD micro-units."
  }
}
variable "assembly_credits" {
  type    = number
  default = 0
  validation {
    condition     = var.assembly_credits >= 0 && floor(var.assembly_credits) == var.assembly_credits
    error_message = "Assembly credits must be a non-negative integer."
  }
}
variable "ffmpeg_path" {
  type    = string
  default = "ffmpeg"
}
variable "migration_database_secret_arn" {
  type        = string
  default     = ""
  description = "Optional owner/migration DSN secret, NEVER supplied to a runtime service."
  validation {
    condition     = var.migration_database_secret_arn == "" || var.migration_database_secret_arn != var.secret_arns["DATABASE_URL"]
    error_message = "Migration and runtime DATABASE_URL must reference different secrets."
  }
}
variable "runtime_database_password_secret_arn" {
  type        = string
  default     = ""
  description = "Optional password secret for explicitly provisioning the restricted database role."
}

variable "premium_price_cents" {
  type    = number
  default = 0
  validation {
    condition     = var.premium_price_cents == 0 || (var.premium_price_cents >= 100 && var.premium_price_cents <= 100000 && floor(var.premium_price_cents) == var.premium_price_cents)
    error_message = "Premium prepaid 30-day USD price must be 0 (disabled) or 100..100000 cents."
  }
}

variable "billing_enabled" {
  type        = bool
  default     = false
  description = "Explicit commercial activation gate; Premium also requires a nonzero reviewed price."
}
