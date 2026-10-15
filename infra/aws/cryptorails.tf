# CryptoRails credentials are API-only Secrets Manager references. Empty defaults
# keep the new checkout path unavailable and never invent a tenant or a wallet.
variable "cryptorails_enabled" {
  type    = bool
  default = false
  validation {
    condition = !var.cryptorails_enabled || (
      var.billing_enabled && var.cryptorails_usd_peg_accepted &&
      contains(keys(var.cryptorails_secret_arns), "CRYPTORAILS_API_KEY") &&
      contains(keys(var.cryptorails_secret_arns), "CRYPTORAILS_WEBHOOK_SECRET") &&
      var.cryptorails_network != "" && var.cryptorails_vm_type != "" &&
      var.cryptorails_token_decimals >= 5 && var.cryptorails_token_decimals <= 18 &&
      var.cryptorails_token != "" && var.cryptorails_token_address != "" &&
      var.cryptorails_treasury_address != "" && var.cryptorails_deposit_fixed != ""
    )
    error_message = "Activation requires billing, explicit stablecoin peg approval, both secret ARNs, reviewed token decimals and exact chain/token/wallet configuration and an explicit fixed fee."
  }
}
variable "cryptorails_secret_arns" {
  type    = map(string)
  default = {}
  validation {
    condition     = alltrue([for key, arn in var.cryptorails_secret_arns : contains(["CRYPTORAILS_API_KEY", "CRYPTORAILS_WEBHOOK_SECRET"], key) && can(regex("^arn:aws:secretsmanager:", arn))])
    error_message = "Only CryptoRails API/webhook secret ARNs belong here; never provide plaintext values."
  }
}
variable "cryptorails_api_url" {
  type    = string
  default = "https://api.cryptorails.tech/v1"
  validation {
    condition     = can(regex("^https://[^/?#]+(/[^?#]*)?$", var.cryptorails_api_url))
    error_message = "Production CryptoRails API URL must use HTTPS without query or fragment."
  }
}
variable "cryptorails_network" {
  type    = string
  default = ""
}
variable "cryptorails_vm_type" {
  type    = string
  default = ""
  validation {
    condition     = contains(["", "EVM", "TRON"], var.cryptorails_vm_type)
    error_message = "Select EVM or TRON after approving the network; blank leaves checkout unconfigured."
  }
}
variable "cryptorails_token" {
  type    = string
  default = ""
  validation {
    condition     = contains(["", "USDC", "USDT"], var.cryptorails_token)
    error_message = "Only explicitly approved USDC/USDT configurations are supported."
  }
}
variable "cryptorails_token_decimals" {
  type    = number
  default = 0
  validation {
    condition     = var.cryptorails_token_decimals == 0 || (var.cryptorails_token_decimals >= 5 && var.cryptorails_token_decimals <= 18 && floor(var.cryptorails_token_decimals) == var.cryptorails_token_decimals)
    error_message = "Token decimals must be explicitly reviewed: 0 disables configuration, or integer 5..18."
  }
}
variable "cryptorails_token_address" {
  type    = string
  default = ""
}
variable "cryptorails_treasury_address" {
  type    = string
  default = ""
}
variable "cryptorails_deposit_fixed" {
  type    = string
  default = ""
  validation {
    condition     = var.cryptorails_deposit_fixed == "" || can(regex("^(0|[1-9][0-9]*)(\\.[0-9]{1,5})?$", var.cryptorails_deposit_fixed))
    error_message = "Declare the exact non-negative fixed token fee as a decimal string (up to 5 decimals), including explicit 0 when approved."
  }
}
variable "cryptorails_deposit_fee_bps" {
  type    = number
  default = 0
  validation {
    condition     = var.cryptorails_deposit_fee_bps >= 0 && var.cryptorails_deposit_fee_bps <= 9999 && floor(var.cryptorails_deposit_fee_bps) == var.cryptorails_deposit_fee_bps
    error_message = "Provider deposit fee must be integer basis points between 0 and 9999."
  }
}
variable "cryptorails_usd_peg_accepted" {
  type        = bool
  default     = false
  description = "Explicit approval of the configured stablecoin USD pricing policy; not an exchange-rate guarantee."
}
locals {
  cryptorails_environment = [
    { name = "CRYPTORAILS_ENABLED", value = tostring(var.cryptorails_enabled) },
    { name = "CRYPTORAILS_API_URL", value = var.cryptorails_api_url },
    { name = "CRYPTORAILS_NETWORK", value = var.cryptorails_network },
    { name = "CRYPTORAILS_VM_TYPE", value = var.cryptorails_vm_type },
    { name = "CRYPTORAILS_TOKEN", value = var.cryptorails_token },
    { name = "CRYPTORAILS_TOKEN_DECIMALS", value = tostring(var.cryptorails_token_decimals) },
    { name = "CRYPTORAILS_TOKEN_ADDRESS", value = var.cryptorails_token_address },
    { name = "CRYPTORAILS_TREASURY_ADDRESS", value = var.cryptorails_treasury_address },
    { name = "CRYPTORAILS_DEPOSIT_FIXED", value = var.cryptorails_deposit_fixed },
    { name = "CRYPTORAILS_DEPOSIT_FEE_BPS", value = tostring(var.cryptorails_deposit_fee_bps) },
    { name = "CRYPTORAILS_USD_PEG_ACCEPTED", value = tostring(var.cryptorails_usd_peg_accepted) }
  ]
}
