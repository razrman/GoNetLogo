# Suggestions form: Lambda behind an HTTP API, mounted at /api/* on the existing
# CloudFront distribution. SES domain + recipient identities are in us-west-2
# (us-east-1 SES is denied by SCP). CloudFront adds a shared origin secret so
# the API is not useful if called directly. A Lambda function URL + OAC cannot
# sign POST bodies unless every browser sends x-amz-content-sha256.

data "archive_file" "contact" {
  type        = "zip"
  source_file = "${path.module}/../lambda/contact.py"
  output_path = "${path.module}/contact.zip"
}

resource "random_password" "contact_origin_secret" {
  length  = 48
  special = false
}

resource "aws_iam_role" "contact" {
  name = "sapienexmachina-landing-contact"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "lambda.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy_attachment" "contact_logs" {
  role       = aws_iam_role.contact.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

resource "aws_iam_role_policy" "contact_ses" {
  name = "ses-send"
  role = aws_iam_role.contact.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = ["ses:SendEmail", "ses:SendRawEmail"]
      Resource = [
        "arn:aws:ses:us-west-2:${data.aws_caller_identity.me.account_id}:identity/${var.domain}",
        "arn:aws:ses:us-west-2:${data.aws_caller_identity.me.account_id}:identity/jphelps@iaip.com",
      ]
    }]
  })
}

resource "aws_cloudwatch_log_group" "contact" {
  name              = "/aws/lambda/sapienexmachina-landing-contact"
  retention_in_days = 30
}

resource "aws_lambda_function" "contact" {
  filename                       = data.archive_file.contact.output_path
  source_code_hash               = data.archive_file.contact.output_base64sha256
  function_name                  = "sapienexmachina-landing-contact"
  role                           = aws_iam_role.contact.arn
  handler                        = "contact.handler"
  runtime                        = "python3.12"
  timeout                        = 10
  memory_size                    = 128
  reserved_concurrent_executions = 2
  architectures                  = ["arm64"]

  environment {
    variables = {
      FROM_ADDR     = "suggestions@${var.domain}"
      TO_ADDR       = "jphelps@iaip.com"
      SES_REGION    = "us-west-2"
      ORIGIN_SECRET = random_password.contact_origin_secret.result
    }
  }

  depends_on = [
    aws_cloudwatch_log_group.contact,
    aws_iam_role_policy_attachment.contact_logs,
    aws_iam_role_policy.contact_ses,
  ]
}

resource "aws_apigatewayv2_api" "contact" {
  name          = "sapienexmachina-landing-contact"
  protocol_type = "HTTP"
  description   = "Landing-page suggestions form"
}

resource "aws_apigatewayv2_integration" "contact" {
  api_id                 = aws_apigatewayv2_api.contact.id
  integration_type       = "AWS_PROXY"
  integration_uri        = aws_lambda_function.contact.invoke_arn
  integration_method     = "POST"
  payload_format_version = "2.0"
}

resource "aws_apigatewayv2_route" "contact" {
  api_id    = aws_apigatewayv2_api.contact.id
  route_key = "POST /api/contact"
  target    = "integrations/${aws_apigatewayv2_integration.contact.id}"
}

resource "aws_apigatewayv2_stage" "contact" {
  api_id      = aws_apigatewayv2_api.contact.id
  name        = "$default"
  auto_deploy = true
}

resource "aws_lambda_permission" "contact_apigw" {
  statement_id  = "AllowAPIGatewayInvoke"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.contact.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.contact.execution_arn}/*/*"
}

# --- SES (us-west-2) ---

resource "aws_ses_domain_identity" "site" {
  domain = var.domain
}

resource "aws_route53_record" "ses_verification" {
  zone_id = var.zone_id
  name    = "_amazonses.${aws_ses_domain_identity.site.domain}"
  type    = "TXT"
  ttl     = 600
  records = [aws_ses_domain_identity.site.verification_token]
}

resource "aws_ses_domain_identity_verification" "site" {
  domain     = aws_ses_domain_identity.site.domain
  depends_on = [aws_route53_record.ses_verification]
}

resource "aws_ses_domain_dkim" "site" {
  domain = aws_ses_domain_identity.site.domain
}

resource "aws_route53_record" "ses_dkim" {
  count   = 3
  zone_id = var.zone_id
  name    = "${aws_ses_domain_dkim.site.dkim_tokens[count.index]}._domainkey.${var.domain}"
  type    = "CNAME"
  ttl     = 600
  records = ["${aws_ses_domain_dkim.site.dkim_tokens[count.index]}.dkim.amazonses.com"]
}

# Sends a verification email to John. SES stays in the sandbox until he clicks it
# (and stays sandboxed after that, which is fine: the only recipient is this address).
resource "aws_ses_email_identity" "john" {
  email = "jphelps@iaip.com"
}

output "contact_function" { value = aws_lambda_function.contact.function_name }
output "contact_log_group" { value = aws_cloudwatch_log_group.contact.name }
output "ses_domain" { value = aws_ses_domain_identity.site.domain }
output "ses_recipient" { value = aws_ses_email_identity.john.email }
