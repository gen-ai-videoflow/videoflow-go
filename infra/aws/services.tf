resource "aws_ecr_repository" "services" {
  for_each             = local.services
  name                 = "${local.name}-${each.key}"
  image_tag_mutability = "IMMUTABLE"
  image_scanning_configuration {
    scan_on_push = true
  }
}
resource "aws_cloudwatch_log_group" "services" {
  for_each          = local.services
  name              = "/ecs/${local.name}/${each.key}"
  retention_in_days = 30
}
resource "aws_ecs_cluster" "main" {
  name = local.name
}
resource "aws_iam_role" "execution" {
  name = "${local.name}-execution"
  assume_role_policy = jsonencode({
    Version = "2012-10-17", Statement = [{
      Effect = "Allow", Principal = {
        Service = "ecs-tasks.amazonaws.com"
      }, Action = "sts:AssumeRole"
    }]
  })
}
resource "aws_iam_role_policy_attachment" "execution" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}
resource "aws_iam_role_policy" "secrets" {
  role = aws_iam_role.execution.id
  policy = jsonencode({
    Version = "2012-10-17", Statement = [{
      Effect = "Allow", Action = ["secretsmanager:GetSecretValue"], Resource = values(var.secret_arns)
    }]
  })
}
resource "aws_iam_role" "task" {
  for_each           = local.services
  name               = "${local.name}-${each.key}"
  assume_role_policy = aws_iam_role.execution.assume_role_policy
}
resource "aws_iam_role_policy" "api" {
  role = aws_iam_role.task["api"].id
  policy = jsonencode({
    Version = "2012-10-17", Statement = [
      {
        Effect = "Allow", Action = ["sqs:SendMessage"], Resource = [for q in aws_sqs_queue.jobs : q.arn]
      },
      {
        Effect = "Allow", Action = ["s3:PutObject"], Resource = ["${aws_s3_bucket.media["uploads"].arn}/*"]
        }, {
        Effect = "Allow", Action = ["s3:GetObject"], Resource = ["${aws_s3_bucket.media["originals"].arn}/*"]
    }]
  })
}
resource "aws_iam_role_policy" "worker" {
  role = aws_iam_role.task["worker"].id
  policy = jsonencode({
    Version = "2012-10-17", Statement = [
      {
        Effect = "Allow", Action = ["sqs:ReceiveMessage", "sqs:DeleteMessage", "sqs:ChangeMessageVisibility", "sqs:GetQueueAttributes", "sqs:SendMessage"], Resource = [for q in aws_sqs_queue.jobs : q.arn]
      },
      {
        Effect = "Allow", Action = ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"], Resource = [for b in aws_s3_bucket.media : "${b.arn}/*"]
      },
      {
        Effect = "Allow", Action = ["mediaconvert:CreateJob", "mediaconvert:GetJob", "mediaconvert:DescribeEndpoints"], Resource = "*"
      },
      {
        Effect = "Allow", Action = ["iam:PassRole"], Resource = aws_iam_role.mediaconvert.arn, Condition = {
          StringEquals = {
            "iam:PassedToService" = "mediaconvert.amazonaws.com"
          }
        }
    }]
  })
}
resource "aws_ecs_task_definition" "services" {
  for_each                 = local.services
  family                   = "${local.name}-${each.key}"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = each.key == "worker" ? "1024" : "256"
  memory                   = each.key == "worker" ? "2048" : "512"
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = aws_iam_role.task[each.key].arn
  container_definitions = jsonencode([{
    name = each.key, image = "${aws_ecr_repository.services[each.key].repository_url}:${var.image_tag}", essential = true,
    portMappings = each.key == "worker" ? [] : [{
      containerPort = 8080, protocol = "tcp"
    }],
    environment = each.key == "web" ? [{
      name = "PORT", value = "8080"
      }, { name = "ORIGIN", value = var.public_origin }, { name = "CLOUD_API_INTERNAL_URL", value = "https://${var.alb_origin_domain}/v1" }] : [
      {
        name = "PORT", value = "8080"
        }, {
        name = "AWS_REGION", value = var.region
        }, {
        name = "CLOUD_DEV_MODE", value = "false"
        }, {
        name = "BILLING_ENABLED", value = "false"
      },
      {
        name = "PUBLIC_ORIGIN", value = var.public_origin
        }, {
        name = "S3_BUCKET_ANALYTICS", value = aws_s3_bucket.media["analytics"].id
        }, {
        name = "S3_BUCKET_UPLOADS", value = aws_s3_bucket.media["uploads"].id
        }, {
        name = "TRUSTED_PROXY_CIDRS", value = var.vpc_cidr
        }, {
        name = "PROVIDER_DAILY_BUDGET_MICROS", value = tostring(var.provider_daily_budget_micros)
        }, {
        name = "PROVIDER_MAX_COST_MICROS", value = tostring(var.provider_max_cost_micros)
        }, {
        name = "OPENROUTER_VIDEO_MODEL", value = var.openrouter_video_model
        }, {
        name = "MAIL_FROM", value = var.ses_from
        }, {
        name = "S3_BUCKET_ORIGINALS", value = aws_s3_bucket.media["originals"].id
      },
      {
        name = "S3_BUCKET_PLAYBACK", value = aws_s3_bucket.media["playback"].id
        }, {
        name = "SQS_QUEUE_URL", value = aws_sqs_queue.jobs["generation"].url
      },
      {
        name = "SQS_DLQ_URL", value = aws_sqs_queue.dlq["generation"].url
        }, {
        name = "ENCODING_QUEUE_URL", value = aws_sqs_queue.jobs["encoding"].url
      },
      {
        name = "MEDIACONVERT_ROLE_ARN", value = aws_iam_role.mediaconvert.arn
        }, {
        name = "MEDIACONVERT_ENDPOINT", value = "https://mediaconvert.${var.region}.amazonaws.com"
      },
      {
        name = "CLOUDFRONT_DOMAIN", value = var.media_domain
        }, {
        name = "CLOUDFRONT_KEY_PAIR_ID", value = aws_cloudfront_public_key.media.id
    }],
    secrets = each.key == "web" ? [] : [for name, arn in var.secret_arns : {
      name = name, valueFrom = arn
    }],
    logConfiguration = {
      logDriver = "awslogs", options = {
        "awslogs-group" = aws_cloudwatch_log_group.services[each.key].name, "awslogs-region" = var.region, "awslogs-stream-prefix" = each.key
      }
    }
  }])
}
resource "aws_ecs_service" "services" {
  for_each                           = local.services
  name                               = "${local.name}-${each.key}"
  cluster                            = aws_ecs_cluster.main.id
  task_definition                    = aws_ecs_task_definition.services[each.key].arn
  desired_count                      = var.desired_count
  launch_type                        = "FARGATE"
  deployment_minimum_healthy_percent = 100
  deployment_maximum_percent         = 200
  network_configuration {
    subnets          = aws_subnet.private[*].id
    security_groups  = [aws_security_group.tasks.id]
    assign_public_ip = false
  }
  dynamic "load_balancer" {
    for_each = each.key == "worker" ? [] : [each.key]
    content {
      target_group_arn = aws_lb_target_group.services[each.key].arn
      container_name   = each.key
      container_port   = 8080
    }
  }
  depends_on = [aws_lb_listener.https]
}
resource "aws_appautoscaling_target" "worker" {
  max_capacity       = var.worker_max_count
  min_capacity       = var.desired_count
  resource_id        = "service/${aws_ecs_cluster.main.name}/${aws_ecs_service.services["worker"].name}"
  scalable_dimension = "ecs:service:DesiredCount"
  service_namespace  = "ecs"
}
resource "aws_lb" "main" {
  name                       = local.name
  load_balancer_type         = "application"
  security_groups            = [aws_security_group.alb.id]
  subnets                    = aws_subnet.public[*].id
  drop_invalid_header_fields = true
}
resource "aws_lb_target_group" "services" {
  for_each    = toset(["api", "web"])
  name        = "${local.name}-${each.key}"
  port        = 8080
  protocol    = "HTTP"
  target_type = "ip"
  vpc_id      = aws_vpc.main.id
  health_check {
    path = each.key == "api" ? "/health" : "/"
  }
}
resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.main.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = var.alb_certificate_arn
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.services["web"].arn
  }
}
resource "aws_lb_listener_rule" "api" {
  listener_arn = aws_lb_listener.https.arn
  priority     = 10
  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.services["api"].arn
  }
  condition {
    path_pattern {
      values = ["/api/*", "/v1/*", "/health", "/authorize"]
    }
  }
}
