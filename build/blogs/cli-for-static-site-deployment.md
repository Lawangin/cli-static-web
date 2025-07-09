# Deploying Static Sites to Multiple Subdomains on S3 with Route 53

It is absolutely possible to build a CLI tool for deploying multiple static web apps to different subdomains hosted on an S3 bucket, with the domain managed in Route 53. Here's a high-level breakdown of how you can achieve this:

Steps to Build the CLI
1. Set Up AWS SDK:

- Use the AWS SDK (e.g., boto3 for Python, aws-sdk for Node.js) to interact with S3, Route 53, and CloudFront (if needed for CDN).
2. Create S3 Buckets for Each Subdomain:

- Each subdomain (e.g., app-a.lawangin.io and app-b.lawangin.io) will have its own S3 bucket configured for static website hosting.
3. Upload Static Files to S3:

- Use the CLI to upload the static files for each app to the corresponding S3 bucket.
4. Configure Route 53 Subdomain Records:

- Add or update Route 53 DNS records to point the subdomains to the S3 bucket endpoints or CloudFront distributions.
5. (Optional) Use CloudFront for CDN:

- If you want to use a CDN for better performance, create CloudFront distributions for each S3 bucket and configure Route 53 to point to the CloudFront distributions.
6. Automate the Process:

Build a CLI tool to automate the above steps for multiple apps.

---

#### Example CLI Workflow
1. Command:
```bash
deploy-static-app --app-name app-a --subdomain app-a.lawangin.io --source ./app-a
```

2. What Happens:

- The CLI uploads the contents of ./app-a to the S3 bucket for app-a.lawangin.io.
- It configures the S3 bucket for static website hosting.
- It updates the Route 53 DNS record to point app-a.lawangin.io to the S3 bucket or CloudFront distribution.

## Prerequisites
1. **AWS Account**: Ensure you have an AWS account with access to S3, Route 53, and optionally CloudFront.
2. **Domain Ownership**: Own a domain (e.g., `lawangin.io`) managed in Route 53.
3. **AWS CLI or SDK**: Install the AWS CLI or SDK for programmatic access.

---

## Steps

### 1. Create S3 Buckets for Each Subdomain
- Each subdomain (e.g., `app-a.lawangin.io`, `app-b.lawangin.io`) requires its own S3 bucket.
- Enable **static website hosting** for each bucket.

#### Example AWS CLI Commands:
```bash
aws s3api create-bucket --bucket app-a.lawangin.io --region us-east-1
aws s3api create-bucket --bucket app-b.lawangin.io --region us-east-1

aws s3 website s3://app-a.lawangin.io/ --index-document [index.html](http://_vscodecontentref_/0) --error-document error.html
aws s3 website s3://app-b.lawangin.io/ --index-document [index.html](http://_vscodecontentref_/1) --error-document error.html
```

### 2. Upload Static Files to S3
- Upload the static files for each app to its respective S3 bucket.

#### Example AWS CLI Command:
```bash
aws s3 sync ./app-a s3://app-a.lawangin.io/
aws s3 sync ./app-b s3://app-b.lawangin.io/
```

---

### 3. Configure Route 53 DNS Records
- Add **CNAME** or **Alias** records in Route 53 to point subdomains to the S3 bucket endpoints.

#### Example AWS CLI Command:
```bash
aws route53 change-resource-record-sets --hosted-zone-id Z1234567890ABCDEF --change-batch '{
  "Changes": [
    {
      "Action": "UPSERT",
      "ResourceRecordSet": {
        "Name": "app-a.lawangin.io",
        "Type": "CNAME",
        "TTL": 300,
        "ResourceRecords": [
          { "Value": "app-a.lawangin.io.s3-website-us-east-1.amazonaws.com" }
        ]
      }
    },
    {
      "Action": "UPSERT",
      "ResourceRecordSet": {
        "Name": "app-b.lawangin.io",
        "Type": "CNAME",
        "TTL": 300,
        "ResourceRecords": [
          { "Value": "app-b.lawangin.io.s3-website-us-east-1.amazonaws.com" }
        ]
      }
    }
  ]
}'
```

---

### 4. Optional: Use CloudFront for CDN and HTTPS
- Create a CloudFront distribution for each S3 bucket to enable HTTPS and improve performance.
- Update Route 53 records to point to the CloudFront distribution instead of the S3 bucket.

#### Example AWS CLI Command:
```bash
aws cloudfront create-distribution --origin-domain-name app-a.lawangin.io.s3.amazonaws.com
aws cloudfront create-distribution --origin-domain-name app-b.lawangin.io.s3.amazonaws.com
```

---

### 5. Automate with a CLI Tool
- Build a CLI tool using AWS SDKs (e.g., `boto3` for Python, `aws-sdk` for Node.js) to automate the above steps.

#### Example CLI Workflow:
```bash
deploy-static-app --app-name app-a --subdomain app-a.lawangin.io --source ./app-a
deploy-static-app --app-name app-b --subdomain app-b.lawangin.io --source ./app-b
```

---

## Result
- `app-a.lawangin.io` and `app-b.lawangin.io` will serve their respective static sites.
- Subdomains will be managed via Route 53, and content will be hosted on S3 (optionally with CloudFront).