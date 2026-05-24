variable "aws_region" {
  description = "AWS region"
  default     = "us-east-1"
}

variable "app_name" {
  description = "Application name prefix for all resources"
  default     = "global-conquest"
}

variable "public_key" {
  description = "SSH public key to install on the instance (contents of your .pub file)"
  type        = string
}
