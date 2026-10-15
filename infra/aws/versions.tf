terraform {
  required_version = ">= 1.11, < 2.0"
  required_providers {
    aws = {
      source = "hashicorp/aws", version = "~> 6.0"
    }
  }
}
provider "aws" {
  region = var.region
}
data "aws_caller_identity" "current" {
}
data "aws_availability_zones" "available" {
  state = "available"
}
locals {
  name     = "videoflow-${var.environment}"
  services = toset(["api", "worker", "web"])
  queues   = toset(["generation", "assembly", "encoding", "analytics"])
}
