# Destroyable app stack: network, load balancer, ECS, logs, and the
# alias record. State is in the bucket from infra/bootstrap, locked with
# OpenTofu's S3 lockfile (use_lockfile). There is no DynamoDB table.
# The bucket name is a literal because a backend block cannot interpolate.
# The hosted zone, certificate, and ECR repository are not in this state.

terraform {
  required_version = ">= 1.10.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "6.72.0"
    }
  }

  backend "s3" {
    bucket       = "gonetlogo-tfstate-457504029722"
    key          = "app/terraform.tfstate"
    region       = "us-west-2"
    encrypt      = true
    use_lockfile = true
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
