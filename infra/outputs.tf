output "alb_dns_name" {
  description = "Backend API + WebSocket URL (use as VITE_API_BASE_URL / VITE_WS_URL)"
  value       = aws_lb.main.dns_name
}

output "s3_website_url" {
  description = "Frontend static website URL"
  value       = "http://${aws_s3_bucket_website_configuration.frontend.website_endpoint}"
}

output "ecr_repository_url" {
  description = "ECR repository URL for the backend image"
  value       = aws_ecr_repository.backend.repository_url
}

output "ecs_cluster_name" {
  value = aws_ecs_cluster.main.name
}

output "ecs_service_name" {
  value = aws_ecs_service.backend.name
}

output "s3_bucket_name" {
  value = aws_s3_bucket.frontend.id
}

output "rds_endpoint" {
  description = "RDS instance hostname (private, only reachable from within the VPC)"
  value       = aws_db_instance.main.address
  sensitive   = true
}
