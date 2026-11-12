variable "aws_region" {
  type    = string
  default = "eu-west-2"
}

variable "localstack" {
  type    = bool
  default = false
}

variable "localstack_endpoint" {
  type    = string
  default = "http://localhost:4566"
}
