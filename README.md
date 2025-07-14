## 🚀 Deploy Static Website with Go CLI

This tool helps you deploy a static website (like a React app) to an S3 bucket, create a CloudFront distribution, and map a subdomain via Route 53, all in one command-line tool.

---

### ✅ Prerequisites

1. **AWS Account** with permissions for S3, CloudFront, ACM, and Route 53
2. **ACM SSL Certificate ARN** for your domain
    - ⚠️ Your certificate **must be in `us-east-1`** for use with CloudFront, even if your other resources are in different regions.
3. **Go installed** if running natively: [https://go.dev/dl/](https://go.dev/dl/)
4. **Docker installed** if running inside a container: [https://docs.docker.com/get-docker/](https://docs.docker.com/get-docker/)
5. `.env` file (used for local dev) or `aws.env` file (used with Docker) with your credentials:

#### Example `aws.env` file
```env
AWS_ACCESS_KEY_ID=your-access-key-id
AWS_SECRET_ACCESS_KEY=your-secret-access-key
AWS_REGION=us-east-1
SSL_CERT_ARN=arn:aws:acm:us-east-1:123456789012:certificate/xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
```

---

### 🧑‍💻 How to Use

#### ▶️ Run Locally (Go CLI)

```bash
go run main.go
```

You'll be prompted to enter:

- Project name (e.g., `myblog`)
- Domain name (e.g., `lawangin.io`)
- Local build folder path (e.g., `./build`)

> 🧠 This will deploy to `myblog.lawangin.io` and upload the contents of `./build`.

---

#### 🐳 Run in Docker

```bash
docker build -t cli-static-web .
docker run --env-file aws.env cli-static-web
```

The CLI will prompt you the same way as the local version.

---

### 🔒 Security Tip

Do **not** bake AWS credentials into the Docker image. Always use `--env-file` or `-e` flags at runtime.

---

### 🧹 Rollback Support

If a failure occurs during deployment (e.g., Route 53 setup fails), the CLI will automatically:

- Delete the CloudFront distribution (if created)
- Delete the S3 bucket (if created)

---

### 📦 Upload Limit

To prevent abuse or accidental misuse, the CLI will reject any folder uploads exceeding **50 MB**.

---

### 🧪 Tested On

- Go 1.22
- Docker 25+
- AWS CLI profile with admin permissions

---
