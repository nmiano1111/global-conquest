variable "aws_region" {
  description = "AWS region"
  default     = "us-east-1"
}

variable "aws_account_id" {
  description = "AWS account ID (used for resource naming)"
}

variable "github_repo" {
  description = "GitHub repository in owner/name format (for OIDC trust)"
}

variable "db_name" {
  description = "PostgreSQL database name"
  default     = "globalconq"
}

variable "db_username" {
  description = "PostgreSQL master username"
  default     = "globalconq"
}

variable "app_name" {
  description = "Application name prefix for all resources"
  default     = "global-conquest"
}
