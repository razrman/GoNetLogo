data "aws_ssm_parameter" "image_tag" {
  name = local.image_tag_parameter
}

data "aws_ecr_repository" "app" {
  name = "gonetlogo"
}

resource "aws_cloudwatch_log_group" "app" {
  name              = "/ecs/gonetlogo"
  retention_in_days = var.log_retention_days
}

resource "aws_ecs_cluster" "app" {
  name = "gonetlogo"

  # Container Insights is a separate charge. The log group is enough.
  setting {
    name  = "containerInsights"
    value = "disabled"
  }
}

resource "aws_ecs_cluster_capacity_providers" "app" {
  cluster_name       = aws_ecs_cluster.app.name
  capacity_providers = ["FARGATE"]

  default_capacity_provider_strategy {
    capacity_provider = "FARGATE"
    weight            = 1
    base              = 1
  }
}

resource "aws_ecs_task_definition" "app" {
  family                   = "gonetlogo"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = tostring(var.cpu)
  memory                   = tostring(var.memory)
  execution_role_arn       = aws_iam_role.execution.arn

  runtime_platform {
    operating_system_family = "LINUX"
    cpu_architecture        = var.cpu_architecture
  }

  container_definitions = jsonencode([
    {
      name      = local.container_name
      image     = "${data.aws_ecr_repository.app.repository_url}:${local.image_tag}"
      essential = true
      # The image entrypoint already passes -addr 0.0.0.0:8080 and
      # -log-format json, so ADDR, PORT, and the log flags are not set.
      # /tmp is the task's ephemeral storage (20 GiB). Distroless has no
      # shell, so there is no container health command; the ALB calls
      # GET /healthz.
      stopTimeout = local.stop_timeout_seconds
      portMappings = [
        {
          containerPort = local.container_port
          hostPort      = local.container_port
          protocol      = "tcp"
        }
      ]
      environment = [
        {
          name  = "GONETLOGO_ALLOWED_ORIGINS"
          value = var.allowed_origins
        },
        {
          name  = "GONETLOGO_MAX_SESSIONS"
          value = tostring(var.max_sessions)
        },
        {
          name  = "GONETLOGO_PRODUCTION"
          value = "1"
        },
        {
          name  = "GONETLOGO_SHUTDOWN_TIMEOUT"
          value = "20s"
        },
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.app.name
          awslogs-region        = var.aws_region
          awslogs-stream-prefix = "ecs"
        }
      }
    }
  ])
}

resource "aws_ecs_service" "app" {
  name                               = "gonetlogo"
  cluster                            = aws_ecs_cluster.app.id
  task_definition                    = aws_ecs_task_definition.app.arn
  desired_count                      = local.desired_count
  launch_type                        = "FARGATE"
  platform_version                   = "1.4.0"
  health_check_grace_period_seconds  = 60
  deployment_minimum_healthy_percent = 100
  deployment_maximum_percent         = 200
  enable_execute_command             = false
  propagate_tags                     = "SERVICE"
  enable_ecs_managed_tags            = true
  wait_for_steady_state              = true

  # A bad rollout returns to the previous task definition.
  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  network_configuration {
    subnets          = aws_subnet.public[*].id
    security_groups  = [aws_security_group.task.id]
    assign_public_ip = true
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.app.arn
    container_name   = local.container_name
    container_port   = local.container_port
  }

  # OpenTofu reads the image tag from SSM, so an apply after a deploy
  # converges on the tag the workflow wrote. Ignore neither the task
  # definition nor the desired count.

  depends_on = [
    aws_lb_listener.http,
    aws_lb_listener.https,
    aws_ecs_cluster_capacity_providers.app,
    aws_iam_role_policy.execution,
  ]
}
