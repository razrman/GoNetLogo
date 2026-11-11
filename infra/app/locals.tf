locals {
  vpc_cidr       = "10.20.0.0/16"
  container_port = 8080
  container_name = "gonetlogo"
  # Must match the parameter created in infra/bootstrap.
  image_tag_parameter = "/gonetlogo/image_tag"
  # Placeholder written by bootstrap. The service stays at 0 tasks until
  # the deploy workflow replaces it with a YYYYMMDD-XX tag.
  # The parameter value is not a secret. The AWS provider marks every
  # SSM read sensitive, which would hide the task definition in plans.
  bootstrap_image_tag = "bootstrap"
  image_tag           = nonsensitive(data.aws_ssm_parameter.image_tag.value)

  # 30s is above the server's 20s default drain (GONETLOGO_SHUTDOWN_TIMEOUT)
  # and within Fargate's stop timeout. The ALB stops sending new requests
  # for the same 30s before ECS signals the task.
  stop_timeout_seconds = 30
  deregistration_delay = 30
  # Websockets. The server pings every 10s and the page heartbeats every 15s.
  alb_idle_timeout = 300

  run_tasks     = local.image_tag != local.bootstrap_image_tag
  desired_count = local.run_tasks ? var.desired_count : 0
}
