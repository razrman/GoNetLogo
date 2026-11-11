variable "aws_region" {
  type        = string
  description = "Region for the state bucket and the app stack."
  default     = "us-west-2"
}

variable "account_id" {
  type        = string
  description = "AWS account that owns this stack. The provider refuses to run anywhere else."
  default     = "457504029722"
}

variable "state_bucket" {
  type        = string
  description = "S3 bucket for infra/app state. The same name is copied into infra/app/versions.tf because a backend block cannot read variables."
  default     = "gonetlogo-tfstate-457504029722"
}

variable "domain_name" {
  type        = string
  description = "Delegated hostname. This stack creates a public hosted zone for this name."
  default     = "gonetlogo.sapienexmachina.si"
}

variable "budget_email" {
  type        = string
  description = "Inbox for the monthly AWS Budgets alert. AWS sends a confirmation link that must be accepted before mail is delivered. The default is the address on this repo's commits."
  default     = "jphelps@iaip.com"
}

variable "budget_limit_usd" {
  type        = string
  description = "Monthly cost alert threshold in USD, measured before credits."
  default     = "20"
}

variable "wait_for_certificate" {
  type        = bool
  description = "Block apply until ACM reports the certificate Issued. Leave false on the first apply so the name servers are printed. Set true after the GoDaddy NS records are in place."
  default     = false
}

variable "github_subject" {
  type        = string
  description = "Exact token.actions.githubusercontent.com:sub that may assume the deploy role. GoNetLogo was created on 2026-10-17, after GitHub started putting owner and repository ids in the subject, so the legacy repo:razrman/GoNetLogo:environment:production string does not match tokens this repo issues."
  default     = "repo:razrman@1226040/GoNetLogo@1423386498:environment:production"
}
