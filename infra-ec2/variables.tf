variable "aws_region" {
  description = "AWS region"
  default     = "us-east-1"
}

variable "app_name" {
  description = "Application name prefix for all resources"
  default     = "global-conquest"
}

variable "my_ip" {
  description = "Your public IP in CIDR notation for SSH access (e.g. 1.2.3.4/32)"
  type        = string
}

variable "public_key" {
  description = "SSH public key to install on the instance (contents of your .pub file)"
  type        = string
}
