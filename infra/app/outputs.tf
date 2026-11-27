output "alb_access_log_bucket" {
  description = "S3 bucket that stores ALB access logs. scripts/top-ips.sh reads it."
  value       = aws_s3_bucket.alb_logs.id
}

output "alb_dns_name" {
  description = "Current load balancer hostname. The Route 53 alias follows this; it changes on every recreate."
  value       = aws_lb.app.dns_name
}

output "site_url" {
  description = "Hostname served by the alias record."
  value       = aws_route53_record.app.fqdn
}

output "ecr_repository_url" {
  description = "Registry the deploy workflow pushes to. The repository survives app teardown."
  value       = data.aws_ecr_repository.app.repository_url
}

output "ecs_cluster" {
  value = aws_ecs_cluster.app.name
}

output "ecs_service" {
  value = aws_ecs_service.app.name
}

output "allowed_cidrs" {
  value = var.allowed_cidrs
}

output "image_tag" {
  description = "Tag SSM currently holds. bootstrap means the service is scaled to zero."
  value       = local.image_tag
}

output "vpn_peering_connection_id" {
  description = "Peering from this VPC to the account default VPC. Empty when peer_openvpn is false."
  value       = try(aws_vpc_peering_connection.openvpn[0].id, "")
}

output "certificate_arn" {
  description = "Issued certificate from the persistent stack, attached to the HTTPS listener."
  value       = data.aws_acm_certificate.app.arn
}
