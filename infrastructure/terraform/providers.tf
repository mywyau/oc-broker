provider "aws" {
  region                      = var.aws_region
  access_key                  = var.localstack ? "test" : null
  secret_key                  = var.localstack ? "test" : null
  skip_credentials_validation = var.localstack
  skip_metadata_api_check     = var.localstack
  skip_requesting_account_id  = var.localstack

  endpoints {
    sqs = var.localstack ? var.localstack_endpoint : null
  }
}
