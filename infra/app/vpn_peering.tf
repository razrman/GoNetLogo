# Peering to the account default VPC, where the OpenVPN server runs.
# razrman/openvpn-infra used to create this connection. It lived outside
# this state, so Infra down could not delete the VPC (DependencyViolation).
# This stack owns the peering: Infra up creates it, and destroy removes
# the routes and then the peering before the VPC.
#
# Direction is this VPC to the default VPC, same account and region.
# auto_accept is enough; there is no second provider. The Name tag stays
# openvpn-gonetlogo so the connection is recognizable. Project and
# ManagedBy come from the provider default_tags.
#
# Destroy order is the graph: each aws_route references the peering, the
# peering references aws_vpc.main, and the public return route references
# aws_route_table.public. Routes go first, then the peering, then the VPC.

data "aws_vpc" "default" {
  count   = var.peer_openvpn ? 1 : 0
  default = true
}

data "aws_route_table" "default_main" {
  count  = var.peer_openvpn ? 1 : 0
  vpc_id = data.aws_vpc.default[0].id

  filter {
    name   = "association.main"
    values = ["true"]
  }
}

# The main table is created with the VPC. Waiting for the public table
# means a same-apply create sees every table that exists today: the main
# table and gonetlogo-public. There is no separate private table.
resource "aws_vpc_peering_connection" "openvpn" {
  count = var.peer_openvpn ? 1 : 0

  vpc_id      = aws_vpc.main.id
  peer_vpc_id = data.aws_vpc.default[0].id
  auto_accept = true

  tags = {
    Name = "openvpn-gonetlogo"
  }

  lifecycle {
    precondition {
      condition     = data.aws_vpc.default[0].cidr_block == "172.31.0.0/16"
      error_message = "Refusing to peer ${data.aws_vpc.default[0].id}: its CIDR is ${data.aws_vpc.default[0].cidr_block}, not 172.31.0.0/16."
    }
  }
}

# The OpenVPN instance subnet uses the default VPC main table.
resource "aws_route" "default_to_app" {
  count = var.peer_openvpn ? 1 : 0

  route_table_id            = data.aws_route_table.default_main[0].id
  destination_cidr_block    = local.vpc_cidr
  vpc_peering_connection_id = aws_vpc_peering_connection.openvpn[0].id
}

# Clients are masqueraded to the instance address in the default VPC, so
# return traffic is 172.31.0.0/16, not the tunnel 10.8.0.0/24.
# Keys are static so plan works on a fresh VPC whose table ids are not
# known yet. Every table in the VPC: the main table and gonetlogo-public.
resource "aws_route" "app_to_default" {
  for_each = var.peer_openvpn ? {
    main   = aws_vpc.main.main_route_table_id
    public = aws_route_table.public.id
  } : {}

  route_table_id            = each.value
  destination_cidr_block    = data.aws_vpc.default[0].cidr_block
  vpc_peering_connection_id = aws_vpc_peering_connection.openvpn[0].id
}
