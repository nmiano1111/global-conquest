#!/usr/bin/env bash
# Run this ONCE before the first `terraform init`.
# Creates the S3 bucket and DynamoDB table that Terraform uses for remote state.

set -euo pipefail

ACCOUNT_ID="294342039804"
REGION="us-east-1"
BUCKET="gc-tfstate-${ACCOUNT_ID}"
TABLE="global-conquest-tfstate-lock"

echo "==> Creating Terraform state bucket: ${BUCKET}"
aws s3api create-bucket \
  --bucket "${BUCKET}" \
  --region "${REGION}"

aws s3api put-bucket-versioning \
  --bucket "${BUCKET}" \
  --versioning-configuration Status=Enabled

aws s3api put-bucket-encryption \
  --bucket "${BUCKET}" \
  --server-side-encryption-configuration \
  '{"Rules":[{"ApplyServerSideEncryptionByDefault":{"SSEAlgorithm":"AES256"}}]}'

aws s3api put-public-access-block \
  --bucket "${BUCKET}" \
  --public-access-block-configuration \
  "BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true"

echo "==> Creating DynamoDB lock table: ${TABLE}"
aws dynamodb create-table \
  --table-name "${TABLE}" \
  --attribute-definitions AttributeName=LockID,AttributeType=S \
  --key-schema AttributeName=LockID,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  --region "${REGION}"

echo ""
echo "Bootstrap complete. You can now run:"
echo "  cd infra && terraform init"
