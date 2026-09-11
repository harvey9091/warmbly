#!/bin/sh
# Create the AWS side of a split deployment: a KMS key, a bucket, a database,
# and the two IAM users that reach them.
#
#   scripts/aws-bootstrap.sh --domain example.com --region eu-central-1
#
# Idempotent: every step checks for what it would create and reports it rather
# than failing, so a re-run after fixing one answer is safe.
#
# The part worth reading is the two policies. A control-plane credential mints
# and opens data keys and reads and writes the bucket; a node credential should
# be able to do neither, and it is the one that ends up on machines in a
# datacentre you share. Warmbly's brokered providers mean a node normally needs
# no AWS credential at all (KMS_PROVIDER=brokered, BLOB_PROVIDER=brokered), so
# --with-node-user is off by default and exists for the deliberate exception.
set -eu

REGION=""
DOMAIN=""
PREFIX="warmbly"
DB_INSTANCE_CLASS="db.t4g.micro"
DB_STORAGE_GB="20"
WITH_NODE_USER="false"
SKIP_DB="false"
DRY_RUN="false"

log()  { printf '%s\n' "$*"; }
warn() { printf '%s\n' "$*" >&2; }
die()  { printf 'error: %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<'USAGE'
Create the AWS resources a split Warmbly deployment needs.

  --domain <d>        Your domain, used to verify an SES identity.      (required)
  --region <r>        AWS region. Put it next to your container host.   (required)
  --prefix <p>        Name prefix for every resource. Default warmbly.
  --db-class <c>      RDS instance class. Default db.t4g.micro.
  --db-storage <gb>   RDS allocated storage in GB. Default 20.
  --skip-db           Create everything except the database.
  --with-node-user    Also create an IAM user for nodes that reach AWS
                      directly. Not needed with the brokered providers.
  --dry-run           Print what would be created, change nothing.
  -h, --help          This text.

Needs the AWS CLI, authenticated as someone who can create KMS keys, S3
buckets, RDS instances and IAM users.
USAGE
}

parse_args() {
  while [ $# -gt 0 ]; do
    case "$1" in
      --domain)          DOMAIN="${2:-}"; shift 2 ;;
      --region)          REGION="${2:-}"; shift 2 ;;
      --prefix)          PREFIX="${2:-}"; shift 2 ;;
      --db-class)        DB_INSTANCE_CLASS="${2:-}"; shift 2 ;;
      --db-storage)      DB_STORAGE_GB="${2:-}"; shift 2 ;;
      --skip-db)         SKIP_DB="true"; shift ;;
      --with-node-user)  WITH_NODE_USER="true"; shift ;;
      --dry-run)         DRY_RUN="true"; shift ;;
      -h|--help)         usage; exit 0 ;;
      *)                 die "unknown option: $1 (try --help)" ;;
    esac
  done
}

require_args() {
  [ -n "$DOMAIN" ] || die "--domain is required"
  [ -n "$REGION" ] || die "--region is required"
  command -v aws >/dev/null 2>&1 || die "the aws CLI is required but not installed"
  ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text) \
    || die "could not read the current AWS identity; is the CLI authenticated?"
  log "Account $ACCOUNT_ID, region $REGION"
}

run() {
  if [ "$DRY_RUN" = "true" ]; then
    log "  would run: $*"
    return 0
  fi
  "$@"
}

# ---- KMS -------------------------------------------------------------------

create_kms() {
  KEY_ALIAS="alias/$PREFIX"
  if aws kms describe-key --key-id "$KEY_ALIAS" --region "$REGION" >/dev/null 2>&1; then
    log "KMS: $KEY_ALIAS already exists"
    KEY_ARN=$(aws kms describe-key --key-id "$KEY_ALIAS" --region "$REGION" \
      --query 'KeyMetadata.Arn' --output text)
    return 0
  fi
  log "KMS: creating $KEY_ALIAS"
  if [ "$DRY_RUN" = "true" ]; then
    log "  would create a symmetric key and alias it $KEY_ALIAS"
    KEY_ARN="arn:aws:kms:$REGION:$ACCOUNT_ID:key/<created>"
    return 0
  fi
  key_id=$(aws kms create-key \
    --description "Warmbly per-organization data keys" \
    --region "$REGION" \
    --query 'KeyMetadata.KeyId' --output text)
  aws kms create-alias --alias-name "$KEY_ALIAS" --target-key-id "$key_id" --region "$REGION"
  KEY_ARN=$(aws kms describe-key --key-id "$key_id" --region "$REGION" \
    --query 'KeyMetadata.Arn' --output text)
  # Losing this key makes every stored mailbox credential unreadable, so it
  # gets the longest window AWS offers against an accidental delete.
  aws kms enable-key-rotation --key-id "$key_id" --region "$REGION" || true
  log "KMS: created $KEY_ALIAS ($key_id)"
}

# ---- S3 --------------------------------------------------------------------

create_bucket() {
  BUCKET="$PREFIX-blobs-$ACCOUNT_ID"
  if aws s3api head-bucket --bucket "$BUCKET" >/dev/null 2>&1; then
    # Bucket names are global, so head-bucket says nothing about WHERE it is.
    # Taking that as "already done" left the blobs in the region of a previous
    # run while everything else moved, which nothing reported.
    actual=$(aws s3api get-bucket-location --bucket "$BUCKET" \
      --query 'LocationConstraint' --output text 2>/dev/null)
    # S3 reports us-east-1 as the literal null, its original default.
    [ "$actual" = "None" ] && actual="us-east-1"
    if [ "$actual" = "$REGION" ]; then
      log "S3: $BUCKET already exists in $REGION"
      return 0
    fi
    die "$BUCKET exists in $actual, not $REGION. A bucket cannot move, and
       deleting one is not this script's call. Empty and delete it, then re-run;
       or pass a different --prefix so this region gets its own bucket."
  fi
  log "S3: creating $BUCKET"
  if [ "$DRY_RUN" = "true" ]; then
    log "  would create $BUCKET with public access blocked and SSE enabled"
    return 0
  fi
  if [ "$REGION" = "us-east-1" ]; then
    aws s3api create-bucket --bucket "$BUCKET" --region "$REGION"
  else
    aws s3api create-bucket --bucket "$BUCKET" --region "$REGION" \
      --create-bucket-configuration "LocationConstraint=$REGION"
  fi
  aws s3api put-public-access-block --bucket "$BUCKET" \
    --public-access-block-configuration \
    "BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true"
  aws s3api put-bucket-encryption --bucket "$BUCKET" \
    --server-side-encryption-configuration \
    '{"Rules":[{"ApplyServerSideEncryptionByDefault":{"SSEAlgorithm":"AES256"}}]}'
  log "S3: created $BUCKET"
}

# ---- IAM -------------------------------------------------------------------

# control_policy is what the backend and consumer need: mint and open data
# keys, read and write the bucket, send platform mail.
#
# Resource is the key ARN, never the alias ARN. IAM does not resolve an alias
# in a Resource element, so a policy naming one grants nothing and the first
# GenerateDataKey fails with AccessDenied against a policy that reads correctly.
control_policy() {
  cat <<POLICY
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "DataKeys",
      "Effect": "Allow",
      "Action": ["kms:GenerateDataKey", "kms:GenerateDataKeyWithoutPlaintext", "kms:Decrypt", "kms:DescribeKey"],
      "Resource": "$KEY_ARN"
    },
    {
      "Sid": "Blobs",
      "Effect": "Allow",
      "Action": ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"],
      "Resource": "arn:aws:s3:::$BUCKET/*"
    },
    {
      "Sid": "BlobsList",
      "Effect": "Allow",
      "Action": ["s3:ListBucket"],
      "Resource": "arn:aws:s3:::$BUCKET"
    },
    {
      "Sid": "PlatformMail",
      "Effect": "Allow",
      "Action": ["ses:SendEmail", "ses:SendRawEmail"],
      "Resource": "*"
    }
  ]
}
POLICY
}

# node_policy is the deliberate exception: a node reaching AWS directly opens
# data keys and moves objects, and mints nothing.
node_policy() {
  cat <<POLICY
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "OpenDataKeys",
      "Effect": "Allow",
      "Action": ["kms:Decrypt"],
      "Resource": "$KEY_ARN"
    },
    {
      "Sid": "Blobs",
      "Effect": "Allow",
      "Action": ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"],
      "Resource": "arn:aws:s3:::$BUCKET/*"
    }
  ]
}
POLICY
}

create_user() {
  user="$1"
  policy_json="$2"
  if aws iam get-user --user-name "$user" >/dev/null 2>&1; then
    log "IAM: $user already exists (policy updated, no new access key)"
  else
    log "IAM: creating $user"
    run aws iam create-user --user-name "$user"
  fi
  if [ "$DRY_RUN" = "true" ]; then
    log "  would attach an inline policy to $user"
    return 0
  fi
  printf '%s' "$policy_json" > "/tmp/$user-policy.json"
  aws iam put-user-policy --user-name "$user" \
    --policy-name "$user-policy" --policy-document "file:///tmp/$user-policy.json"
  rm -f "/tmp/$user-policy.json"
}

create_users() {
  [ -n "${KEY_ARN:-}" ] || die "internal: the key ARN was not resolved before the policies were built"
  create_user "$PREFIX-control" "$(control_policy)"
  if [ "$WITH_NODE_USER" = "true" ]; then
    create_user "$PREFIX-node" "$(node_policy)"
  else
    log "IAM: skipping the node user; nodes use the brokered providers and need no AWS credential"
  fi
}

# ---- RDS -------------------------------------------------------------------

create_db() {
  if [ "$SKIP_DB" = "true" ]; then
    log "RDS: skipped"
    return 0
  fi
  DB_ID="$PREFIX-db"
  if aws rds describe-db-instances --db-instance-identifier "$DB_ID" --region "$REGION" >/dev/null 2>&1; then
    log "RDS: $DB_ID already exists"
    return 0
  fi
  log "RDS: creating $DB_ID ($DB_INSTANCE_CLASS, ${DB_STORAGE_GB}GB)"
  if [ "$DRY_RUN" = "true" ]; then
    log "  would create a publicly accessible Postgres 16 instance with storage encrypted"
    return 0
  fi
  DB_PASSWORD=$(openssl rand -base64 30 | tr -d '/+=' | cut -c1-28)
  aws rds create-db-instance \
    --db-instance-identifier "$DB_ID" \
    --db-instance-class "$DB_INSTANCE_CLASS" \
    --engine postgres \
    --engine-version 16 \
    --allocated-storage "$DB_STORAGE_GB" \
    --storage-type gp3 \
    --storage-encrypted \
    --master-username warmbly \
    --master-user-password "$DB_PASSWORD" \
    --db-name warmbly \
    --backup-retention-period 7 \
    --publicly-accessible \
    --no-multi-az \
    --region "$REGION" >/dev/null
  # Never stdout: that is a terminal, a CI log or an agent transcript, and the
  # value cannot be rotated back out of any of them.
  pwfile="./${PREFIX}-db-password.txt"
  ( umask 077; printf '%s\n' "$DB_PASSWORD" > "$pwfile" )
  chmod 600 "$pwfile"
  DB_PASSWORD_FILE="$pwfile"
  log "RDS: creating. The master password was written to $pwfile (mode 0600)."
}

# ---- SES -------------------------------------------------------------------

create_ses_identity() {
  if aws sesv2 get-email-identity --email-identity "$DOMAIN" --region "$REGION" >/dev/null 2>&1; then
    log "SES: $DOMAIN is already an identity"
    return 0
  fi
  log "SES: creating a domain identity for $DOMAIN"
  if [ "$DRY_RUN" = "true" ]; then
    log "  would create an SES domain identity with Easy DKIM"
    return 0
  fi
  aws sesv2 create-email-identity --email-identity "$DOMAIN" --region "$REGION" >/dev/null
  log "SES: publish the DKIM records it now expects:"
  aws sesv2 get-email-identity --email-identity "$DOMAIN" --region "$REGION" \
    --query 'DkimAttributes.Tokens' --output text 2>/dev/null || true
}

summary() {
  log ""
  log "Done. What to put in the control plane's environment:"
  log ""
  log "  AWS_REGION=$REGION"
  log "  KMS_PROVIDER=aws"
  log "  KMS_AWS_KEY_ID=alias/$PREFIX"
  log "  BLOB_PROVIDER=s3"
  log "  BLOB_BUCKET=${BUCKET:-<bucket>}"
  log ""
  log "Still to do by hand, because each one is a decision rather than a default:"
  log ""
  log "  1. an access key for $PREFIX-control, into the control plane's environment"
  log "  2. the RDS security group: allow 5432 from your container host and your"
  log "     own address, not from everywhere"
  log "  3. rds.force_ssl=1 in the instance's parameter group"
  log "  4. the DKIM records above, in DNS"
  log "  5. SES production access; a sandboxed account only delivers to verified"
  log "     addresses"
  if [ -n "${DB_PASSWORD_FILE:-}" ]; then
    log ""
    log "  The database master password is in $DB_PASSWORD_FILE."
    log "  Move it into your secret store and delete the file; this script"
    log "  cannot show it again, and it was never written to this output."
  fi
  log ""
}

main() {
  parse_args "$@"
  require_args
  create_kms
  create_bucket
  create_users
  create_db
  create_ses_identity
  summary
  return 0
}

main "$@"
