resource "aws_budgets_budget" "monthly" {
  name         = local.name
  budget_type  = "COST"
  limit_amount = tostring(var.monthly_budget_usd)
  limit_unit   = "USD"
  time_unit    = "MONTHLY"
  dynamic "notification" {
    for_each = [50, 75, 90, 100]
    content {
      comparison_operator        = "GREATER_THAN"
      threshold                  = notification.value
      threshold_type             = "PERCENTAGE"
      notification_type          = "ACTUAL"
      subscriber_email_addresses = [var.budget_email]
    }
  }
}
resource "aws_cloudwatch_metric_alarm" "dlq" {
  for_each            = local.queues
  alarm_name          = "${local.name}-${each.key}-dlq"
  namespace           = "AWS/SQS"
  metric_name         = "ApproximateNumberOfMessagesVisible"
  statistic           = "Maximum"
  period              = 300
  evaluation_periods  = 1
  threshold           = 0
  comparison_operator = "GreaterThanThreshold"
  dimensions = {
    QueueName = aws_sqs_queue.dlq[each.key].name
  }
  treat_missing_data = "notBreaching"
}
output "ecr_repositories" {
  value = {
    for k, v in aws_ecr_repository.services : k => v.repository_url
  }
}
output "alb_dns_name" {
  value = aws_lb.main.dns_name
}
output "database_endpoint" {
  value = aws_db_instance.main.endpoint
}
output "database_admin_secret_arn" {
  value = aws_db_instance.main.master_user_secret[0].secret_arn
}
output "redis_endpoint" {
  value = aws_elasticache_replication_group.main.primary_endpoint_address
}
output "queues" {
  value = {
    for k, v in aws_sqs_queue.jobs : k => v.url
  }
}
output "media_distribution_id" {
  value = aws_cloudfront_distribution.media.id
}
output "web_distribution_domain" {
  value = aws_cloudfront_distribution.web.domain_name
}
output "media_distribution_domain" {
  value = aws_cloudfront_distribution.media.domain_name
}
output "buckets" {
  value = {
    for k, v in aws_s3_bucket.media : k => v.id
  }
}

resource "aws_iam_role_policy" "email" {
  count = var.ses_identity == "" ? 0 : 1
  role  = aws_iam_role.task["api"].id
  policy = jsonencode({ Version = "2012-10-17", Statement = [{
    Effect    = "Allow", Action = ["ses:SendEmail", "ses:SendRawEmail"],
    Resource  = "arn:aws:ses:${var.region}:${data.aws_caller_identity.current.account_id}:identity/${var.ses_identity}",
    Condition = { StringEquals = { "ses:FromAddress" = var.ses_from } }
  }] })
}
