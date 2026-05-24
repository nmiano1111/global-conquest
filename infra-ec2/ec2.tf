data "aws_ami" "ubuntu" {
  most_recent = true
  owners      = ["099720109477"] # Canonical

  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-*"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

resource "aws_key_pair" "main" {
  key_name   = var.app_name
  public_key = var.public_key
  tags       = { Name = var.app_name }
}

resource "aws_instance" "main" {
  ami                    = data.aws_ami.ubuntu.id
  instance_type          = "t3a.micro"
  key_name               = aws_key_pair.main.key_name
  vpc_security_group_ids = [aws_security_group.ec2.id]
  iam_instance_profile   = aws_iam_instance_profile.ec2.name

  user_data = <<-EOF
    #!/bin/bash
    set -e

    apt-get update
    apt-get install -y ca-certificates curl

    install -m 0755 -d /etc/apt/keyrings
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
    chmod a+r /etc/apt/keyrings/docker.asc

    echo \
      "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] \
      https://download.docker.com/linux/ubuntu \
      $(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
      | tee /etc/apt/sources.list.d/docker.list > /dev/null

    apt-get update
    apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin awscli

    usermod -aG docker ubuntu

    mkdir -p /home/ubuntu/deploy
    chown ubuntu:ubuntu /home/ubuntu/deploy
  EOF

  user_data_replace_on_change = true

  tags = { Name = var.app_name }
}

resource "aws_eip" "main" {
  instance = aws_instance.main.id
  domain   = "vpc"
  tags     = { Name = var.app_name }
}
