resource "random_password" "db" {
  length  = 32
  special = false
}

resource "aws_ssm_parameter" "db_password" {
  name  = "/${var.app_name}/db_password"
  type  = "SecureString"
  value = random_password.db.result
}

resource "aws_db_subnet_group" "main" {
  name       = var.app_name
  subnet_ids = aws_subnet.private[*].id
}

resource "aws_security_group" "rds" {
  name   = "${var.app_name}-rds"
  vpc_id = aws_vpc.main.id

  ingress {
    description     = "PostgreSQL from ECS tasks"
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [aws_security_group.ecs.id]
  }

  tags = { Name = "${var.app_name}-rds" }
}

resource "aws_db_instance" "main" {
  identifier     = var.app_name
  engine         = "postgres"
  engine_version = "16"

  # Smallest available instance class for PostgreSQL
  instance_class    = "db.t4g.micro"
  allocated_storage = 20
  storage_type      = "gp2"

  db_name  = var.db_name
  username = var.db_username
  password = random_password.db.result

  db_subnet_group_name   = aws_db_subnet_group.main.name
  vpc_security_group_ids = [aws_security_group.rds.id]

  publicly_accessible = false
  multi_az            = false

  # Safe to skip for a test deployment; set to true before going to production
  skip_final_snapshot = true
  deletion_protection = false

  tags = { Name = var.app_name }
}
