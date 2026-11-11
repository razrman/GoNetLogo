# Resources that stay while the app stack is destroyed: the image
# repository, the delegated DNS zone, and the certificate. The validation
# CNAME is written in this zone, so it does not change across teardowns
# and nothing is copied into GoDaddy except the zone's name servers, once.

resource "aws_ecr_repository" "app" {
  name                 = "gonetlogo"
  image_tag_mutability = "MUTABLE"
  force_delete         = true

  image_scanning_configuration {
    scan_on_push = true
  }
}

resource "aws_ecr_lifecycle_policy" "app" {
  repository = aws_ecr_repository.app.name

  policy = jsonencode({
    rules = [
      {
        rulePriority = 1
        description  = "Keep the last 10 images"
        selection = {
          tagStatus   = "any"
          countType   = "imageCountMoreThan"
          countNumber = 10
        }
        action = {
          type = "expire"
        }
      }
    ]
  })
}

resource "aws_route53_zone" "app" {
  name          = var.domain_name
  comment       = "Delegated from GoDaddy. The parent apex and www stay at GoDaddy."
  force_destroy = true

  tags = {
    Name = var.domain_name
  }
}

resource "aws_acm_certificate" "app" {
  domain_name       = var.domain_name
  validation_method = "DNS"

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_route53_record" "cert_validation" {
  zone_id         = aws_route53_zone.app.zone_id
  name            = tolist(aws_acm_certificate.app.domain_validation_options)[0].resource_record_name
  type            = tolist(aws_acm_certificate.app.domain_validation_options)[0].resource_record_type
  ttl             = 300
  records         = [tolist(aws_acm_certificate.app.domain_validation_options)[0].resource_record_value]
  allow_overwrite = true
}

resource "aws_acm_certificate_validation" "app" {
  count                   = var.wait_for_certificate ? 1 : 0
  certificate_arn         = aws_acm_certificate.app.arn
  validation_record_fqdns = [aws_route53_record.cert_validation.fqdn]

  timeouts {
    create = "45m"
  }
}

# Usage cost, with credits left out, so the alert still fires while the
# free-plan credit balance is paying the bill. The first two budgets in
# an account are free. AWS will not deliver mail until the address
# accepts the confirmation link.
resource "aws_budgets_budget" "monthly" {
  name         = "gonetlogo-monthly"
  budget_type  = "COST"
  limit_amount = var.budget_limit_usd
  limit_unit   = "USD"
  time_unit    = "MONTHLY"

  cost_types {
    include_credit             = false
    include_discount           = true
    include_other_subscription = true
    include_recurring          = true
    include_refund             = false
    include_subscription       = true
    include_support            = true
    include_tax                = true
    include_upfront            = true
    use_blended                = false
  }

  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 100
    threshold_type             = "PERCENTAGE"
    notification_type          = "ACTUAL"
    subscriber_email_addresses = [var.budget_email]
  }

  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 100
    threshold_type             = "PERCENTAGE"
    notification_type          = "FORECASTED"
    subscriber_email_addresses = [var.budget_email]
  }
}
