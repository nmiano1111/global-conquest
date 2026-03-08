# Deployment Guide

Plain HTTP on AWS, no custom domain required.

```
Browser → S3 static website (frontend)
            └── calls/connects to ALB DNS name (HTTP)
                    └── ECS Fargate (Go backend)
                            └── RDS PostgreSQL (private subnet)
```

---

## Prerequisites

- AWS CLI configured (`aws configure`) with admin-level permissions
- Terraform >= 1.5 (`brew install terraform`)
- Docker
- Node 20

---

## Step 1 — Bootstrap Terraform state (one time only)

```bash
cd infra
bash bootstrap.sh
```

This creates an S3 bucket and DynamoDB table for Terraform remote state.

---

## Step 2 — Provision infrastructure

```bash
cd infra
terraform init
terraform apply
```

This creates:
- VPC, subnets, Internet Gateway
- ECR repository
- RDS PostgreSQL (`db.t4g.micro`, private subnet)
- ECS Fargate cluster + service
- Application Load Balancer (HTTP port 80)
- S3 bucket for the frontend
- IAM roles (ECS task, GitHub Actions OIDC)
- SSM parameter for DB password

Note the outputs — you'll need `alb_dns_name` and `s3_website_url`.

After the first apply, the ECS service will show as unhealthy until the first
Docker image is pushed (Step 3).

---

## Step 3 — First deploy

Push a commit to `main`. The `Deploy` GitHub Actions workflow will:

1. Build the Go backend Docker image and push it to ECR
2. Register a new ECS task definition revision and update the service
3. Wait for ECS to stabilise
4. Build the React frontend with the ALB URL baked in
5. Sync the build to S3

Alternatively, trigger it manually from the Actions tab (`workflow_dispatch`).

---

## Step 4 — Access the app

```
Frontend:  http://<s3_website_url>         (from terraform output)
API:       http://<alb_dns_name>/api/ping
```

---

## GitHub Actions setup

The workflow uses OIDC — no AWS secrets needed in GitHub.

The only required setting: go to your repo → **Settings → Actions → General**
and ensure "Allow GitHub Actions to create and approve pull requests" is on
(so the OIDC token request works).

The role `global-conquest-github-actions` is created by Terraform and trusts
the repo `nmiano1111/global-conquest`.

---

## Re-deploying

- **Infrastructure changes**: edit files in `infra/`, push to `main`.
  The `Terraform` workflow runs automatically.
- **App changes**: push to `main`. The `Deploy` workflow runs automatically.

---

## Tearing everything down

```bash
cd infra
terraform destroy
```

Then delete the Terraform state bucket manually if desired:
```bash
aws s3 rb s3://gc-tfstate-294342039804 --force
aws dynamodb delete-table --table-name global-conquest-tfstate-lock
```

---

## Cost estimate (us-east-1)

| Resource | $/month |
|---|---|
| RDS db.t4g.micro | ~$13 |
| ECS Fargate (256 CPU / 512 MB, 24/7) | ~$9 |
| ALB | ~$16 |
| S3 + data transfer | ~$1 |
| ECR storage | ~$0.50 |
| **Total** | **~$40** |

The ALB is the biggest fixed cost. Tear it down when not in use to save money.
