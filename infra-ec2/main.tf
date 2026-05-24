terraform {
  required_version = ">= 1.5"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }

  backend "s3" {
    bucket         = "gc-tfstate-294342039804"
    key            = "global-conquest/ec2/terraform.tfstate"
    region         = "us-east-1"
    dynamodb_table = "global-conquest-tfstate-lock"
    encrypt        = true
  }
}

provider "aws" {
  region = var.aws_region
}

data "aws_vpc" "default" {
  default = true
}
