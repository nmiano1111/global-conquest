terraform {
  required_version = ">= 1.5"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.0"
    }
  }

  # State is stored in S3. Run infra/bootstrap.sh once before the first
  # `terraform init` to create the bucket and DynamoDB lock table.
  backend "s3" {
    bucket         = "gc-tfstate-294342039804"
    key            = "global-conquest/terraform.tfstate"
    region         = "us-east-1"
    dynamodb_table = "global-conquest-tfstate-lock"
    encrypt        = true
  }
}

provider "aws" {
  region = var.aws_region
}
