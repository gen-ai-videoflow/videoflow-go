resource "aws_iam_role" "mediaconvert" {
  name = "${local.name}-mediaconvert"
  assume_role_policy = jsonencode({
    Version = "2012-10-17", Statement = [{
      Effect = "Allow", Principal = {
        Service = "mediaconvert.amazonaws.com"
      }, Action = "sts:AssumeRole"
    }]
  })
}
resource "aws_iam_role_policy" "mediaconvert" {
  role = aws_iam_role.mediaconvert.id
  policy = jsonencode({
    Version = "2012-10-17", Statement = [
      {
        Effect = "Allow", Action = ["s3:GetObject"], Resource = "${aws_s3_bucket.media["originals"].arn}/*"
      },
      {
        Effect = "Allow", Action = ["s3:PutObject"], Resource = "${aws_s3_bucket.media["playback"].arn}/*"
      },
      {
        Effect = "Allow", Action = ["s3:ListBucket", "s3:GetBucketLocation"], Resource = [aws_s3_bucket.media["originals"].arn, aws_s3_bucket.media["playback"].arn]
    }]
  })
}
resource "aws_cloudwatch_event_rule" "encoding" {
  name = "${local.name}-encoding"
  event_pattern = jsonencode({
    source = ["aws.mediaconvert"], "detail-type" = ["MediaConvert Job State Change"], detail = {
      status = ["COMPLETE", "ERROR", "CANCELED"]
    }
  })
}
resource "aws_cloudwatch_event_target" "encoding" {
  rule = aws_cloudwatch_event_rule.encoding.name
  arn  = aws_sqs_queue.jobs["encoding"].arn
  dead_letter_config {
    arn = aws_sqs_queue.dlq["encoding"].arn
  }
  retry_policy {
    maximum_event_age_in_seconds = 86400
    maximum_retry_attempts       = 185
  }
}
resource "aws_sqs_queue_policy" "events" {
  for_each  = toset(["jobs", "dlq"])
  queue_url = each.key == "jobs" ? aws_sqs_queue.jobs["encoding"].url : aws_sqs_queue.dlq["encoding"].url
  policy = jsonencode({
    Version = "2012-10-17", Statement = [{
      Effect = "Allow", Principal = {
        Service = "events.amazonaws.com"
        }, Action = "sqs:SendMessage", Resource = each.key == "jobs" ? aws_sqs_queue.jobs["encoding"].arn : aws_sqs_queue.dlq["encoding"].arn, Condition = {
        ArnEquals = {
          "aws:SourceArn" = aws_cloudwatch_event_rule.encoding.arn
        }
      }
    }]
  })
}
resource "aws_cloudfront_public_key" "media" {
  name        = "${local.name}-media"
  encoded_key = var.cloudfront_public_key_pem
}
resource "aws_cloudfront_key_group" "media" {
  name  = "${local.name}-media"
  items = [aws_cloudfront_public_key.media.id]
}
resource "aws_cloudfront_origin_access_control" "media" {
  name                              = "${local.name}-media"
  origin_access_control_origin_type = "s3"
  signing_behavior                  = "always"
  signing_protocol                  = "sigv4"
}
data "aws_cloudfront_cache_policy" "disabled" {
  name = "Managed-CachingDisabled"
}
data "aws_cloudfront_cache_policy" "optimized" {
  name = "Managed-CachingOptimized"
}
resource "aws_cloudfront_origin_request_policy" "viewer" {
  name = "${local.name}-viewer-and-edge-identity"
  cookies_config { cookie_behavior = "all" }
  query_strings_config { query_string_behavior = "all" }
  headers_config {
    header_behavior = "allViewerAndWhitelistCloudFront"
    headers { items = ["CloudFront-Viewer-Address", "CloudFront-Forwarded-Proto"] }
  }
}
resource "aws_cloudfront_distribution" "media" {
  enabled = true
  aliases = [var.media_domain]
  origin {
    domain_name              = aws_s3_bucket.media["playback"].bucket_regional_domain_name
    origin_id                = "playback"
    origin_access_control_id = aws_cloudfront_origin_access_control.media.id
  }
  origin {
    domain_name = var.alb_origin_domain
    origin_id   = "api"
    custom_origin_config {
      http_port              = 80
      https_port             = 443
      origin_protocol_policy = "https-only"
      origin_ssl_protocols   = ["TLSv1.2"]
    }
  }
  default_cache_behavior {
    target_origin_id           = "playback"
    viewer_protocol_policy     = "redirect-to-https"
    allowed_methods            = ["GET", "HEAD", "OPTIONS"]
    cached_methods             = ["GET", "HEAD"]
    cache_policy_id            = data.aws_cloudfront_cache_policy.optimized.id
    trusted_key_groups         = [aws_cloudfront_key_group.media.id]
    response_headers_policy_id = aws_cloudfront_response_headers_policy.media.id
  }
  ordered_cache_behavior {
    path_pattern             = "/authorize"
    target_origin_id         = "api"
    viewer_protocol_policy   = "https-only"
    allowed_methods          = ["GET", "HEAD", "OPTIONS"]
    cached_methods           = ["GET", "HEAD"]
    cache_policy_id          = data.aws_cloudfront_cache_policy.disabled.id
    origin_request_policy_id = aws_cloudfront_origin_request_policy.viewer.id
  }
  ordered_cache_behavior {
    path_pattern             = "/api/*"
    target_origin_id         = "api"
    viewer_protocol_policy   = "https-only"
    allowed_methods          = ["GET", "HEAD", "OPTIONS", "PUT", "POST", "PATCH", "DELETE"]
    cached_methods           = ["GET", "HEAD"]
    cache_policy_id          = data.aws_cloudfront_cache_policy.disabled.id
    origin_request_policy_id = aws_cloudfront_origin_request_policy.viewer.id
  }
  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }
  viewer_certificate {
    acm_certificate_arn      = var.edge_certificate_arn
    ssl_support_method       = "sni-only"
    minimum_protocol_version = "TLSv1.2_2021"
  }
}
resource "aws_cloudfront_response_headers_policy" "media" {
  name = "${local.name}-media"
  cors_config {
    access_control_allow_credentials = true
    access_control_allow_headers {
      items = ["Range", "Content-Type"]
    }
    access_control_allow_methods {
      items = ["GET", "HEAD", "OPTIONS"]
    }
    access_control_allow_origins {
      items = [var.public_origin]
    }
    access_control_expose_headers {
      items = ["Content-Length", "Content-Range", "Accept-Ranges"]
    }
    origin_override = true
  }
}
resource "aws_s3_bucket_policy" "media" {
  for_each = aws_s3_bucket.media
  bucket   = each.value.id
  policy = jsonencode({
    Version = "2012-10-17", Statement = concat([
      {
        Sid = "DenyInsecureTransport", Effect = "Deny", Principal = "*", Action = "s3:*", Resource = [each.value.arn, "${each.value.arn}/*"], Condition = {
          Bool = {
            "aws:SecureTransport" = "false"
          }
        }
      }], each.key == "playback" ? [
      {
        Effect = "Allow", Principal = {
          Service = "cloudfront.amazonaws.com"
          }, Action = "s3:GetObject", Resource = "${each.value.arn}/*", Condition = {
          StringEquals = {
            "AWS:SourceArn" = aws_cloudfront_distribution.media.arn
          }
        }
    }] : [])
  })
}
resource "aws_cloudfront_distribution" "web" {
  enabled = true
  aliases = [var.web_domain]
  origin {
    domain_name = var.alb_origin_domain
    origin_id   = "alb"
    custom_origin_config {
      http_port              = 80
      https_port             = 443
      origin_protocol_policy = "https-only"
      origin_ssl_protocols   = ["TLSv1.2"]
    }
  }
  default_cache_behavior {
    target_origin_id         = "alb"
    viewer_protocol_policy   = "redirect-to-https"
    allowed_methods          = ["GET", "HEAD", "OPTIONS", "PUT", "POST", "PATCH", "DELETE"]
    cached_methods           = ["GET", "HEAD"]
    cache_policy_id          = data.aws_cloudfront_cache_policy.disabled.id
    origin_request_policy_id = aws_cloudfront_origin_request_policy.viewer.id
  }
  ordered_cache_behavior {
    path_pattern           = "/_app/immutable/*"
    target_origin_id       = "alb"
    viewer_protocol_policy = "redirect-to-https"
    allowed_methods        = ["GET", "HEAD"]
    cached_methods         = ["GET", "HEAD"]
    cache_policy_id        = data.aws_cloudfront_cache_policy.optimized.id
    compress               = true
  }
  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }
  viewer_certificate {
    acm_certificate_arn      = var.edge_certificate_arn
    ssl_support_method       = "sni-only"
    minimum_protocol_version = "TLSv1.2_2021"
  }
}
