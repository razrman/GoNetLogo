output "state_bucket" {
  description = "S3 bucket holding infra/app state. The app backend block uses this name."
  value       = aws_s3_bucket.state.bucket
}

output "deploy_role_arn" {
  description = "Value for the GitHub repository variable AWS_DEPLOY_ROLE_ARN."
  value       = aws_iam_role.deploy.arn
}

output "github_oidc_provider_arn" {
  description = "IAM OIDC provider for token.actions.githubusercontent.com."
  value       = aws_iam_openid_connect_provider.github.arn
}

output "github_subject" {
  description = "Only OIDC subject allowed to assume the deploy role."
  value       = var.github_subject
}

output "image_tag_parameter" {
  description = "SSM parameter the deploy workflow overwrites and infra/app reads."
  value       = aws_ssm_parameter.image_tag.name
}

output "name_servers" {
  description = "Add these four values at GoDaddy as NS records for host gonetlogo. One time. Leave the apex and www records alone."
  value       = aws_route53_zone.app.name_servers
}

output "hosted_zone_id" {
  description = "Route 53 zone that survives app teardown."
  value       = aws_route53_zone.app.zone_id
}

output "certificate_arn" {
  description = "ACM certificate in us-west-2. The app stack attaches it after ACM reports Issued."
  value       = aws_acm_certificate.app.arn
}

output "ecr_repository_url" {
  description = "Image repository. Images survive app teardown."
  value       = aws_ecr_repository.app.repository_url
}

output "budget_email" {
  description = "Address that must confirm the AWS Budgets subscription."
  value       = var.budget_email
}
