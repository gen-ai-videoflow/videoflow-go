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
    condition     = alltrue([for key in ["DATABASE_URL", "REDIS_URL", "STRIPE_SECRET_KEY", "STRIPE_WEBHOOK_SECRET", "CLOUDFRONT_PRIVATE_KEY", "OPENROUTER_API_KEY"] : contains(keys(var.secret_arns), key)]) && alltrue([for arn in values(var.secret_arns) : can(regex("^arn:aws:secretsmanager:", arn))])
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
# secret_arns must include DATABASE_URL, REDIS_URL, STRIPE_SECRET_KEY,
# STRIPE_WEBHOOK_SECRET, CLOUDFRONT_PRIVATE_KEY. Values are full secret ARNs.
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
