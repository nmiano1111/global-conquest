# EC2 Migration Plan — Minimal Cost Deployment

> **Goal:** Replace ECS Fargate + ALB + RDS (~$53/mo) with a single EC2 instance running
> Docker Compose + Caddy + local Postgres (~$8/mo).
>
> The existing `infra/` Terraform setup is preserved untouched in git. When the game goes
> public, `terraform apply` from `infra/` recreates the full production stack.

---

## Cost Impact

| Resource | Current | After |
|---|---|---|
| ECS Fargate (1 task, 256 CPU / 512 MB) | ~$18/mo | gone |
| Application Load Balancer | ~$17/mo | gone |
| RDS db.t4g.micro (PostgreSQL 16) | ~$14/mo | gone |
| EC2 t3a.micro | — | ~$7/mo |
| S3, ECR | ~$4/mo | ~$2/mo |
| **Total** | **~$53/mo** | **~$9/mo** |

---

## Key Decisions

### t3a.micro (amd64) instead of t4g.micro (arm64)
The existing Dockerfile builds for `linux/amd64` (the default on GitHub's ubuntu-latest
runners). Using a t3a.micro avoids any cross-compilation changes — one less thing to
break. The price difference is $0.72/month vs t4g.micro; not worth the friction.

### Postgres in Docker instead of RDS
RDS is a managed service that costs ~$14/month even when idle. For a small testing
deployment, a Postgres container on the EC2 instance with a named Docker volume is
sufficient. Flyway already handles migrations automatically on startup. The tradeoff is
that backups are manual (see Operational Notes below), but for a friends-testing
environment this is acceptable.

### HTTP-only (no domain required)
The current setup already uses plain HTTP — the ALB runs on port 80 and the frontend
bakes `ws://` URLs. The EC2 setup maintains this. Caddy is included but runs in HTTP
mode, acting as a reverse proxy on port 80. Adding a domain later is a one-line change
to the Caddyfile, and Caddy handles TLS automatically.

### `infra/` is already "saved"
The production Terraform files are in git. No copies or archives are needed. Live AWS
resources will be destroyed via `terraform destroy` once the EC2 setup is validated.
Recreating the production stack later is just `cd infra && terraform apply`.

---

## What Changes, What Doesn't

| Component | Action |
|---|---|
| `infra/` Terraform files | **Untouched.** Lives in git. Resources destroyed at end of migration. |
| ECR repository | **Kept.** Same image registry, same CI push flow. |
| S3 frontend bucket | **Kept.** Same static hosting. Only the baked-in `VITE_*` URLs change. |
| GitHub OIDC IAM role | **Pruned.** Remove ECS/ALB permissions. Add EC2 describe. |
| ECS cluster / service | **Destroyed** after EC2 is validated. |
| ALB | **Destroyed.** Replaced by Caddy on EC2. |
| RDS | **Destroyed.** Replaced by Postgres in a Docker volume on EC2. |

---

## New Files

```
infra-ec2/
  main.tf              # provider + S3 backend (key: global-conquest/ec2/terraform.tfstate)
  ec2.tf               # t3a.micro, Elastic IP, key pair, user-data (installs Docker)
  security_groups.tf   # ingress: 22 (your IP), 80, 443
  iam.tf               # EC2 instance profile → ECR pull permissions
  variables.tf
  outputs.tf           # elastic_ip output

deploy/
  docker-compose.prod.yml   # caddy + backend (ECR image) + postgres + flyway
  Caddyfile                  # HTTP mode; comment swap enables HTTPS when domain added
  .env.example               # DB_PASSWORD, ECR_IMAGE, CORS_ALLOWED_ORIGIN

.github/workflows/
  deploy.yml    # modified: replace ECS steps with SSH deploy; fix VITE_* URLs
  infra.yml     # modified: paths trigger → infra-ec2/**, working-dir → infra-ec2
```

No changes to `backend/Dockerfile`.

---

## Implementation Steps

### Step 1 — Create `infra-ec2/` Terraform

Provisions:
- **EC2 t3a.micro** in `us-east-1a` with Ubuntu 24.04 (amd64)
- **Elastic IP** attached to the instance (free while running)
- **Security group:** SSH/22 restricted to your IP, HTTP/80 and HTTPS/443 open
- **IAM instance profile** with ECR pull permissions
  (`ecr:GetAuthorizationToken`, `ecr:BatchGetImage`, `ecr:GetDownloadUrlForLayer`)
- **User data script:** installs Docker CE + Compose plugin on first boot
- **Key pair:** AWS-generated; download the PEM file and add to GitHub secrets
- **Terraform state key:** `global-conquest/ec2/terraform.tfstate` (same S3 bucket,
  no conflict with existing state)

### Step 2 — Create `deploy/docker-compose.prod.yml`

Based on `backend/docker-compose.yml`. Changes:
- `globalconq-service` renamed to `backend`; uses `image: ${ECR_IMAGE}` instead of `build:`
- **Caddy service** added: `caddy:2-alpine`, ports `80:80` and `443:443`,
  mounts Caddyfile + two named volumes (`caddy_data`, `caddy_config`)
- `restart: unless-stopped` on postgres and backend
- Flyway stays one-shot (`restart: no`)
- Postgres DB password comes from `.env` (`${DB_PASSWORD}`) instead of hardcoded
- Backend does **not** expose port 8080 to the host — Caddy reaches it over the
  internal Docker network

### Step 3 — Create `deploy/Caddyfile`

```
# HTTP-only (no domain required):
:80 {
    reverse_proxy backend:8080
}

# To enable HTTPS later, replace the block above with:
# risk.yourdomain.com {
#     reverse_proxy backend:8080
# }
```

Caddy 2 handles WebSocket proxy upgrades correctly by default — no extra directives needed.

### Step 4 — Update `deploy.yml` (CI/CD)

ECR build + push steps stay **identical**. Changes:

- **Remove** the "Update ECS service" step
- **Add** SSH deploy step:
  1. Write `EC2_SSH_KEY` secret to `/tmp/key.pem`
  2. SSH into `EC2_HOST` (Elastic IP, stored as GitHub secret)
  3. Run: ECR login → `docker compose pull backend` → `docker compose up -d`
- **Frontend job:** remove "Get ALB DNS name" step; replace with fixed env vars:
  ```
  VITE_API_BASE_URL: "http://${{ vars.EC2_HOST }}/api"
  VITE_WS_URL:       "ws://${{ vars.EC2_HOST }}/ws"
  ```

New GitHub secrets required: `EC2_SSH_KEY`, `EC2_HOST`

### Step 5 — Update `infra.yml`

Change paths trigger from `infra/**` to `infra-ec2/**`  
Change `working-directory` from `infra` to `infra-ec2`

### Step 6 — Apply new infra

```bash
cd infra-ec2
terraform init
terraform apply
```

Note the Elastic IP from outputs. Docker is installed and ready.

### Step 7 — First manual deploy

```bash
# Copy secrets to the instance
scp -i key.pem deploy/.env ubuntu@<elastic-ip>:~/deploy/.env

# SSH in and start the stack
ssh -i key.pem ubuntu@<elastic-ip>
cd ~/deploy
docker compose -f docker-compose.prod.yml up -d

# Verify
curl http://localhost/api/ping
```

### Step 8 — Validate

Push a new commit to trigger the CI deploy. Confirm:
- `GET /api/ping` returns 200
- WebSocket connects and game state updates flow
- Run a full test game

### Step 9 — Destroy old infra

Once the EC2 setup is validated and friends have tested it:

```bash
cd infra
terraform destroy
```

Destroys: ECS cluster/service/task definition, ALB, RDS, VPC, CloudWatch log group,
SSM parameter. Does **not** touch: `infra/` Terraform files, ECR repo, S3 bucket, IAM role.

### Step 10 — IAM cleanup

Remove the now-unused ECS and ALB permissions from the GitHub Actions OIDC role.
This can be done via the AWS console or a targeted `terraform apply` on just `iam.tf`.

---

## Database Migration Note

For a testing environment, starting fresh is simplest — Flyway runs all migrations
automatically on first boot. If existing game data needs to be preserved:

```bash
# 1. Dump from RDS (run from any machine with DB access)
pg_dump -h <rds-endpoint> -U globalconq -d globalconq --no-owner > dump.sql

# 2. Copy to EC2
scp -i key.pem dump.sql ubuntu@<elastic-ip>:~/

# 3. Restore into the Docker container (after stack is running)
docker exec -i postgres psql -U globalconq -d globalconq < ~/dump.sql
```

---

## Operational Notes

| Task | Command |
|---|---|
| Restart all services | `docker compose -f docker-compose.prod.yml restart` |
| View backend logs | `docker compose -f docker-compose.prod.yml logs -f backend` |
| View Caddy logs | `docker compose -f docker-compose.prod.yml logs -f caddy` |
| Deploy new image | Automatic via CI on push to `main` |
| Manual redeploy | SSH in → `docker compose pull backend && docker compose up -d` |
| DB backup | `docker exec postgres pg_dump -U globalconq globalconq > backup_$(date +%F).sql` |
| TLS cert renewal | Automatic — Caddy renews certs before expiry; no action needed |
| Upgrade EC2 size | Stop instance → change type in `infra-ec2/ec2.tf` → `terraform apply` |

---

## Enabling HTTPS Later

When you have a domain:

1. Create an **A record** pointing `risk.yourdomain.com` → the Elastic IP
2. Edit `deploy/Caddyfile` — swap the `:80` block for the domain block (see Step 3 above)
3. Update `VITE_API_BASE_URL` and `VITE_WS_URL` in `deploy.yml` to use `https://` and `wss://`
4. `docker compose restart caddy` on the EC2 instance — Caddy fetches the cert automatically
