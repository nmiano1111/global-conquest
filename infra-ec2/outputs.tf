output "elastic_ip" {
  description = "Public IP of the EC2 instance — use this for DNS and GitHub Actions vars"
  value       = aws_eip.main.public_ip
}

output "instance_id" {
  value = aws_instance.main.id
}
