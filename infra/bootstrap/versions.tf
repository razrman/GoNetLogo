# Bootstrap is applied once, by hand, with an administrator key.
# State stays on that machine (there is no backend block). The app stack
# in infra/app then uses the bucket this stack creates.
#
# OpenTofu 1.10 or newer is required by the app stack's S3 lockfile.
# Workflows install the version in infra/.opentofu-version.

terraform {
  required_version = ">= 1.10.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "6.72.0"
    }
  }
}

provider "aws" {
  region              = var.aws_region
  allowed_account_ids = [var.account_id]

  default_tags {
    tags = {
      Project   = "gonetlogo"
      ManagedBy = "opentofu"
    }
  }
}
