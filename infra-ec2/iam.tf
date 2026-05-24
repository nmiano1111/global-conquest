resource "aws_iam_role" "ec2" {
  name = "${var.app_name}-ec2"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action    = "sts:AssumeRole"
      Effect    = "Allow"
      Principal = { Service = "ec2.amazonaws.com" }
    }]
  })

  tags = { Name = "${var.app_name}-ec2" }
}

resource "aws_iam_role_policy" "ec2_ecr" {
  name = "${var.app_name}-ec2-ecr"
  role = aws_iam_role.ec2.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = [
        "ecr:GetAuthorizationToken",
        "ecr:BatchGetImage",
        "ecr:GetDownloadUrlForLayer",
        "ecr:BatchCheckLayerAvailability",
      ]
      Resource = "*"
    }]
  })
}

resource "aws_iam_instance_profile" "ec2" {
  name = "${var.app_name}-ec2"
  role = aws_iam_role.ec2.name
}
