# CloudFront standard logging (v2) to S3. The delivery source has to be
# created in us-east-1. The bucket stays in us-west-2 with the site.
# Object ownership stays BucketOwnerPreferred because the delivery
# service still sends an bucket-owner-full-control ACL. force_destroy
# lets a teardown remove a bucket that still has logs.

variable "access_log_retention_days" {
  type    = number
  default = 90
}

resource "aws_s3_bucket" "cf_logs" {
  bucket        = "sapienexmachina-cf-logs-${data.aws_caller_identity.me.account_id}"
  force_destroy = true
}

resource "aws_s3_bucket_public_access_block" "cf_logs" {
  bucket = aws_s3_bucket.cf_logs.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_ownership_controls" "cf_logs" {
  bucket = aws_s3_bucket.cf_logs.id

  rule {
    object_ownership = "BucketOwnerPreferred"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "cf_logs" {
  bucket = aws_s3_bucket.cf_logs.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "cf_logs" {
  bucket = aws_s3_bucket.cf_logs.id

  rule {
    id     = "expire-access-logs"
    status = "Enabled"

    filter {}

    expiration {
      days = var.access_log_retention_days
    }

    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
  }
}

data "aws_iam_policy_document" "cf_logs" {
  statement {
    sid    = "AWSLogsDeliveryWrite"
    effect = "Allow"

    principals {
      type        = "Service"
      identifiers = ["delivery.logs.amazonaws.com"]
    }

    actions   = ["s3:PutObject"]
    resources = ["${aws_s3_bucket.cf_logs.arn}/*"]

    condition {
      test     = "StringEquals"
      variable = "s3:x-amz-acl"
      values   = ["bucket-owner-full-control"]
    }

    condition {
      test     = "StringEquals"
      variable = "aws:SourceAccount"
      values   = [data.aws_caller_identity.me.account_id]
    }

    condition {
      test     = "ArnLike"
      variable = "aws:SourceArn"
      values   = ["arn:aws:logs:us-east-1:${data.aws_caller_identity.me.account_id}:delivery-source:*"]
    }
  }
}

resource "aws_s3_bucket_policy" "cf_logs" {
  bucket = aws_s3_bucket.cf_logs.id
  policy = data.aws_iam_policy_document.cf_logs.json

  depends_on = [
    aws_s3_bucket_public_access_block.cf_logs,
    aws_s3_bucket_ownership_controls.cf_logs,
  ]
}

resource "aws_cloudwatch_log_delivery_source" "cf" {
  provider = aws.use1

  name         = "sapienexmachina-landing"
  log_type     = "ACCESS_LOGS"
  resource_arn = aws_cloudfront_distribution.site.arn
}

resource "aws_cloudwatch_log_delivery_destination" "cf" {
  provider = aws.use1

  name          = "sapienexmachina-landing"
  output_format = "json"

  delivery_destination_configuration {
    destination_resource_arn = aws_s3_bucket.cf_logs.arn
  }
}

resource "aws_cloudwatch_log_delivery" "cf" {
  provider = aws.use1

  delivery_source_name     = aws_cloudwatch_log_delivery_source.cf.name
  delivery_destination_arn = aws_cloudwatch_log_delivery_destination.cf.arn

  depends_on = [aws_s3_bucket_policy.cf_logs]
}

output "cf_access_log_bucket" {
  value = aws_s3_bucket.cf_logs.id
}
