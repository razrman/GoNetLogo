data "aws_availability_zones" "available" {
  state = "available"

  # Skip Local Zones. An ALB subnet has to be in a standard AZ.
  filter {
    name   = "opt-in-status"
    values = ["opt-in-not-required"]
  }
}

resource "aws_vpc" "main" {
  cidr_block           = local.vpc_cidr
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = {
    Name = "gonetlogo"
  }
}

resource "aws_internet_gateway" "main" {
  vpc_id = aws_vpc.main.id

  tags = {
    Name = "gonetlogo"
  }
}

resource "aws_subnet" "public" {
  count = 2

  vpc_id                  = aws_vpc.main.id
  cidr_block              = cidrsubnet(local.vpc_cidr, 8, count.index)
  availability_zone       = data.aws_availability_zones.available.names[count.index]
  map_public_ip_on_launch = true

  tags = {
    Name = "gonetlogo-public-${count.index}"
  }

  # Destroy waits while Fargate releases elastic network interfaces.
  timeouts {
    delete = "20m"
  }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.main.id

  tags = {
    Name = "gonetlogo-public"
  }
}

resource "aws_route" "public_default" {
  route_table_id         = aws_route_table.public.id
  destination_cidr_block = "0.0.0.0/0"
  gateway_id             = aws_internet_gateway.main.id
}

resource "aws_route_table_association" "public" {
  count = 2

  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id
}

# Inline egress replaces the default allow-all rule. The load balancer
# does not reference the task group, so the two groups are not a cycle.
# The task group is what limits port 8080 to the load balancer.
resource "aws_security_group" "alb" {
  name                   = "gonetlogo-alb"
  description            = "Load balancer. Ingress is allowed_cidrs on 80 and 443."
  vpc_id                 = aws_vpc.main.id
  revoke_rules_on_delete = true

  ingress {
    description = "HTTP from allowed clients"
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = var.allowed_cidrs
  }

  ingress {
    description = "HTTPS from allowed clients"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = var.allowed_cidrs
  }

  egress {
    description = "To tasks in this VPC"
    from_port   = local.container_port
    to_port     = local.container_port
    protocol    = "tcp"
    cidr_blocks = [local.vpc_cidr]
  }

  tags = {
    Name = "gonetlogo-alb"
  }

  timeouts {
    delete = "20m"
  }
}

resource "aws_security_group" "task" {
  name                   = "gonetlogo-task"
  description            = "Fargate task. Ingress is the load balancer only."
  vpc_id                 = aws_vpc.main.id
  revoke_rules_on_delete = true

  ingress {
    description     = "Load balancer to the container"
    from_port       = local.container_port
    to_port         = local.container_port
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
  }

  # Public address, no NAT. ECR, CloudWatch Logs, and ECS are HTTPS.
  egress {
    description = "HTTPS to ECR, CloudWatch Logs, and ECS"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "gonetlogo-task"
  }

  timeouts {
    delete = "20m"
  }
}
