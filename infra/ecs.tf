resource "aws_ecs_cluster" "main" {
  name = var.app_name
  tags = { Name = var.app_name }
}

resource "aws_cloudwatch_log_group" "backend" {
  name              = "/ecs/${var.app_name}-backend"
  retention_in_days = 7
}

resource "aws_ecs_task_definition" "backend" {
  family                   = "${var.app_name}-backend"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = 256
  memory                   = 512
  execution_role_arn       = aws_iam_role.ecs_task_execution.arn
  task_role_arn            = aws_iam_role.ecs_task.arn

  container_definitions = jsonencode([{
    name  = "backend"
    # On first `terraform apply` there is no image yet; the ECS service will
    # show unhealthy until the first CI/CD deploy pushes an image.
    image = "${aws_ecr_repository.backend.repository_url}:latest"

    portMappings = [{
      containerPort = 8080
      protocol      = "tcp"
    }]

    environment = [
      { name = "DB_HOST", value = aws_db_instance.main.address },
      { name = "DB_PORT", value = "5432" },
      { name = "DB_USER", value = var.db_username },
      { name = "DB_NAME", value = var.db_name },
      # Allow WebSocket connections from any origin (HTTP-only test deployment)
      { name = "WS_ALLOWED_ORIGINS", value = "*" },
      { name = "DB_SSL_MODE", value = "require" },
    ]

    secrets = [{
      name      = "DB_PASSWORD"
      valueFrom = aws_ssm_parameter.db_password.arn
    }]

    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.backend.name
        "awslogs-region"        = var.aws_region
        "awslogs-stream-prefix" = "backend"
      }
    }
  }])
}

resource "aws_ecs_service" "backend" {
  name            = "${var.app_name}-backend"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.backend.arn
  desired_count   = 1
  launch_type     = "FARGATE"

  network_configuration {
    # ECS tasks sit in public subnets with public IPs so they can reach ECR
    # and CloudWatch without a NAT gateway.
    subnets          = aws_subnet.public[*].id
    security_groups  = [aws_security_group.ecs.id]
    assign_public_ip = true
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.backend.arn
    container_name   = "backend"
    container_port   = 8080
  }

  depends_on = [aws_lb_listener.http]

  lifecycle {
    # CI/CD manages the task definition revision after the initial deploy.
    ignore_changes = [task_definition]
  }
}
