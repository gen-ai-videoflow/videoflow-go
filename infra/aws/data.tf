resource "aws_db_subnet_group" "main" {
  subnet_ids = aws_subnet.private[*].id
}
resource "aws_db_instance" "main" {
  identifier                  = local.name
  engine                      = "postgres"
  instance_class              = "db.t4g.micro"
  allocated_storage           = 20
  max_allocated_storage       = 100
  storage_encrypted           = true
  db_name                     = "videoflow"
  username                    = "videoflow_admin"
  manage_master_user_password = true
  db_subnet_group_name        = aws_db_subnet_group.main.name
  vpc_security_group_ids      = [aws_security_group.data.id]
  publicly_accessible         = false
  multi_az                    = var.multi_az
  backup_retention_period     = 14
  deletion_protection         = true
  skip_final_snapshot         = false
  final_snapshot_identifier   = "${local.name}-final"
  auto_minor_version_upgrade  = true
}
# Ephemeral reads avoid persisting the raw Redis token in Terraform state.
ephemeral "aws_secretsmanager_secret_version" "redis" {
  secret_id = var.redis_auth_secret_arn
}
resource "aws_elasticache_subnet_group" "main" {
  name       = local.name
  subnet_ids = aws_subnet.private[*].id
}
resource "aws_elasticache_replication_group" "main" {
  replication_group_id       = local.name
  description                = "Disposable shared coordination"
  engine                     = "redis"
  engine_version             = "7.1"
  node_type                  = "cache.t4g.micro"
  num_cache_clusters         = var.multi_az ? 2 : 1
  automatic_failover_enabled = var.multi_az
  multi_az_enabled           = var.multi_az
  port                       = 6379
  subnet_group_name          = aws_elasticache_subnet_group.main.name
  security_group_ids         = [aws_security_group.data.id]
  at_rest_encryption_enabled = true
  transit_encryption_enabled = true
  auth_token_wo              = ephemeral.aws_secretsmanager_secret_version.redis.secret_string
  auth_token_wo_version      = 1
  parameter_group_name       = aws_elasticache_parameter_group.main.name
}
resource "aws_elasticache_parameter_group" "main" {
  name   = local.name
  family = "redis7"
  parameter {
    name  = "maxmemory-policy"
    value = "noeviction"
  }
}
resource "aws_s3_bucket" "media" {
  for_each = toset(["originals", "playback", "analytics", "uploads"])
  bucket   = "${local.name}-${data.aws_caller_identity.current.account_id}-${each.key}"
}
resource "aws_s3_bucket_public_access_block" "media" {
  for_each                = aws_s3_bucket.media
  bucket                  = each.value.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}
resource "aws_s3_bucket_server_side_encryption_configuration" "media" {
  for_each = aws_s3_bucket.media
  bucket   = each.value.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}
resource "aws_s3_bucket_versioning" "originals" {
  bucket = aws_s3_bucket.media["originals"].id
  versioning_configuration {
    status = "Enabled"
  }
}
resource "aws_s3_bucket_lifecycle_configuration" "media" {
  for_each = aws_s3_bucket.media
  bucket   = each.value.id
  rule {
    id     = "abort-incomplete"
    status = "Enabled"
    filter {
      prefix = ""
    }
    abort_incomplete_multipart_upload {
      days_after_initiation = 1
    }
  }
  dynamic "rule" {
    for_each = each.key == "uploads" ? [1] : []
    content {
      id     = "upload-quarantine-expiry"
      status = "Enabled"
      filter { prefix = "" }
      expiration { days = 1 }
    }
  }
  dynamic "rule" {
    for_each = each.key == "originals" ? [1] : []
    content {
      id     = "bounded-noncurrent-versions"
      status = "Enabled"
      filter { prefix = "" }
      noncurrent_version_expiration { noncurrent_days = 30 }
    }
  }
  rule {
    id     = "temporary"
    status = "Enabled"
    filter {
      prefix = "tmp/"
    }
    expiration {
      days = 1
    }
  }
}
resource "aws_sqs_queue" "dlq" {
  for_each                  = local.queues
  name                      = "${local.name}-${each.key}-dlq"
  message_retention_seconds = 1209600
  sqs_managed_sse_enabled   = true
}
resource "aws_sqs_queue" "jobs" {
  for_each                   = local.queues
  name                       = "${local.name}-${each.key}"
  visibility_timeout_seconds = 300
  receive_wait_time_seconds  = 20
  sqs_managed_sse_enabled    = true
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.dlq[each.key].arn, maxReceiveCount = 5
  })
}
resource "aws_sqs_queue_redrive_allow_policy" "dlq" {
  for_each  = local.queues
  queue_url = aws_sqs_queue.dlq[each.key].url
  redrive_allow_policy = jsonencode({
    redrivePermission = "byQueue", sourceQueueArns = [aws_sqs_queue.jobs[each.key].arn]
  })
}
resource "aws_s3_bucket_cors_configuration" "uploads" {
  bucket = aws_s3_bucket.media["uploads"].id
  cors_rule {
    allowed_origins = [var.public_origin]
    allowed_methods = ["POST", "HEAD"]
    allowed_headers = ["*"]
    expose_headers  = ["ETag"]
    max_age_seconds = 300
  }
}
