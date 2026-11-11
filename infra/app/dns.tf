data "aws_route53_zone" "app" {
  name         = var.zone_name
  private_zone = false
}

# A name inside the delegated zone, not the zone apex. Other apps can
# take other names in the same zone. Recreating the load balancer changes
# its DNS name; this alias is updated in the same apply, so GoDaddy stays
# untouched. IPv4 only: ingress is an IPv4 allow-list, and an AAAA record
# would advertise an address that list does not accept.
resource "aws_route53_record" "app" {
  zone_id = data.aws_route53_zone.app.zone_id
  name    = var.app_hostname
  type    = "A"

  lifecycle {
    precondition {
      condition     = var.app_hostname == var.zone_name || endswith(var.app_hostname, ".${var.zone_name}")
      error_message = "app_hostname must be zone_name or a name under it."
    }
  }

  alias {
    name    = aws_lb.app.dns_name
    zone_id = aws_lb.app.zone_id
    # One record, no second target to fail over to. An unhealthy task
    # still resolves so the load balancer can answer.
    evaluate_target_health = false
  }
}
