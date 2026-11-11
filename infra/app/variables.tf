variable "aws_region" {
  type        = string
  description = "Region for every regional resource. The backend block in versions.tf repeats this."
  default     = "us-west-2"
}

variable "account_id" {
  type        = string
  description = "AWS account. The provider refuses to run anywhere else."
  default     = "457504029722"
}

variable "zone_name" {
  type        = string
  description = "Hosted zone from infra/bootstrap. The alias is created in this zone."
  default     = "app.sapienexmachina.si"
}

variable "app_hostname" {
  type        = string
  description = "Alias name, and the issued certificate this stack looks up. A name under zone_name."
  default     = "gonetlogo.app.sapienexmachina.si"
}

variable "allowed_origins" {
  type        = string
  description = "GONETLOGO_ALLOWED_ORIGINS. Same-origin browser calls are allowed even when this is empty, because the ALB preserves Host."
  default     = "https://gonetlogo.app.sapienexmachina.si"
}

variable "allowed_cidrs" {
  type        = list(string)
  description = "IPv4 CIDRs allowed to open the load balancer on ports 80 and 443. The default is public. Edit this and apply to lock the load balancer down again."
  default     = ["0.0.0.0/0"]

  validation {
    condition = length(var.allowed_cidrs) > 0 && alltrue([
      for cidr in var.allowed_cidrs : can(cidrhost(cidr, 0))
    ])
    error_message = "allowed_cidrs must contain at least one IPv4 CIDR."
  }
}

variable "max_sessions" {
  type        = number
  description = "GONETLOGO_MAX_SESSIONS. Sessions live in memory on the one task."
  default     = 32

  validation {
    condition     = var.max_sessions >= 0
    error_message = "max_sessions must be zero or positive. Zero means unlimited."
  }
}

variable "cpu" {
  type        = number
  description = "Fargate task CPU units. 512 is 0.5 vCPU. With 512, memory may be 1024, 2048, 3072, or 4096."
  default     = 512
}

variable "memory" {
  type        = number
  description = "Fargate task memory in MiB. 1024 is the 0.5 vCPU minimum."
  default     = 1024
}

variable "cpu_architecture" {
  type        = string
  description = "Fargate CPU architecture. ARM64 is Graviton. The deploy workflow pushes both amd64 and arm64."
  default     = "ARM64"

  validation {
    condition     = contains(["ARM64", "X86_64"], var.cpu_architecture)
    error_message = "cpu_architecture must be ARM64 or X86_64."
  }
}

variable "desired_count" {
  type        = number
  description = "Tasks to run once an image tag exists. Sessions are in memory, so more than 1 splits clients across tasks."
  default     = 1

  validation {
    condition     = var.desired_count >= 0 && var.desired_count <= 2
    error_message = "desired_count must be 0, 1, or 2. Stay at 1 unless sessions move out of the process."
  }
}

variable "log_retention_days" {
  type        = number
  description = "CloudWatch log retention for /ecs/gonetlogo."
  default     = 14
}
