package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/joho/godotenv"

	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"

	"github.com/aws/aws-sdk-go-v2/service/route53"
	r53types "github.com/aws/aws-sdk-go-v2/service/route53/types"

	"mime"
	"os"
	"path/filepath"
)

func main() {
	if os.Getenv("AWS_REGION") == "" {
		err := godotenv.Load() // fallback for local .env
		if err != nil {
			log.Fatalf("Error loading .env file: %v", err)
		}

	}

	awsRegion := os.Getenv("AWS_REGION")
	sslCertARN := os.Getenv("SSL_CERT_ARN")
	if awsRegion == "" || sslCertARN == "" {
		log.Fatalf("Missing required environment variables: AWS_REGION or SSL_CERT_ARN")
	}

	var projectName string
	fmt.Print("Enter project name (e.g., myblog): ")
	_, err := fmt.Scanln(&projectName)
	if err != nil {
		log.Fatalf("project name not valid, %v", err)
	}

	var domain string
	fmt.Print("Enter domain name (e.g., mywebsite.com): ")
	_, err = fmt.Scanln(&domain)
	if err != nil {
		log.Fatalf("project name not valid, %v", err)
	}

	bucketName := fmt.Sprintf("%s.%s", projectName, domain)

	var folderPath string
	fmt.Print("Enter local path to build folder (e.g., ./build): ")
	_, err = fmt.Scanln(&folderPath)
	if err != nil {
		log.Fatalf("local path not valid, %v", err)
	}

	const maxUploadSizeBytes = 50 * 1024 * 1024 // 50 MB

	size, err := folderSize(folderPath)
	if err != nil {
		log.Fatalf("Failed to check folder size: %v", err)
	}
	if size > maxUploadSizeBytes {
		log.Fatalf("Build folder exceeds max allowed size (%.2f MB)", float64(size)/1024.0/1024.0)
	}

	ctx := context.TODO()

	s3Created := false
	cfCreated := false
	var cfDistID string

	// Load the Shared AWS Configuration (~/.aws/config)
	log.Println("Loading configuration...")
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(awsRegion))
	if err != nil {
		log.Fatalf("failed to load configuration, %v", err)
	}

	// Create an Amazon S3 service client
	log.Println("Creating s3 client...")
	client := s3.NewFromConfig(cfg)

	defer func() {
		if err != nil {
			log.Println("Error occurred, starting rollback...")
			if cfCreated {
				cfClient := cloudfront.NewFromConfig(cfg)
				deleteCloudFrontDistribution(ctx, cfClient, cfDistID)
			}
			if s3Created {
				deleteS3Bucket(ctx, client, bucketName)
			}
			log.Fatalf("Deployment failed: %v", err)
		}
	}()

	// Get the first page of results for ListObjectsV2 for a bucket
	bucketsList, err := client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		log.Fatal(err)
	}

	exists := false
	for _, b := range bucketsList.Buckets {
		if aws.ToString(b.Name) == bucketName {
			exists = true
			break
		}
	}

	if !exists {
		log.Println("Creating Bucket...")
		_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: aws.String(bucketName),
		})
		if err != nil {
			err = fmt.Errorf("Bucket creation failed: %w", err)
			return
		}
		log.Println("Bucket created:", bucketName)
		s3Created = true
	} else {
		log.Println("Bucket already exists:", bucketName)
	}

	// enable s3 bucket to utilize static web hosting
	_, err = client.PutBucketWebsite(ctx, &s3.PutBucketWebsiteInput{
		Bucket: aws.String(bucketName),
		WebsiteConfiguration: &s3types.WebsiteConfiguration{
			IndexDocument: &s3types.IndexDocument{Suffix: aws.String("index.html")},
			ErrorDocument: &s3types.ErrorDocument{Key: aws.String("index.html")},
		},
	})
	if err != nil {
		err = fmt.Errorf("Failed to enable static website: %w", err)
		return
	}

	// remove blocked access to bucket
	_, err = client.PutPublicAccessBlock(ctx, &s3.PutPublicAccessBlockInput{
		Bucket: aws.String(bucketName),
		PublicAccessBlockConfiguration: &s3types.PublicAccessBlockConfiguration{
			BlockPublicAcls:       aws.Bool(false),
			IgnorePublicAcls:      aws.Bool(false),
			BlockPublicPolicy:     aws.Bool(false),
			RestrictPublicBuckets: aws.Bool(false),
		},
	})
	if err != nil {
		err = fmt.Errorf("Failed to enable public policy: %w", err)
	}

	// define bucket policy
	policy := map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Sid":       "PublicReadGetObject",
				"Effect":    "Allow",
				"Principal": "*",
				"Action":    "s3:GetObject",
				"Resource":  fmt.Sprintf("arn:aws:s3:::%s/*", bucketName),
			},
		},
	}

	policyJson, _ := json.Marshal(policy)

	// apply bucket policy
	_, err = client.PutBucketPolicy(context.Background(), &s3.PutBucketPolicyInput{
		Bucket: aws.String(bucketName),
		Policy: aws.String(string(policyJson)),
	})
	if err != nil {
		err = fmt.Errorf("Failed to enable bucket policy: %w", err)
		return
	}

	err = uploadFolder(client, bucketName, folderPath, "")
	if err != nil {
		err = fmt.Errorf("Failed to upload folder: %w", err)
		return
	}

	log.Printf("Site available at: http://%s.s3-website-%s.amazonaws.com\n", bucketName, awsRegion)

	cfClient := cloudfront.NewFromConfig(cfg)

	cfDomain, distID, err := createCloudFrontDistribution(
		ctx,
		cfClient,
		bucketName,
		fmt.Sprintf("%s.s3-website-%s.amazonaws.com", bucketName, awsRegion),
		sslCertARN,
	)
	if err != nil {
		err = fmt.Errorf("Failed to create CloudFront Domain: %w", err)
		return
	}
	log.Printf("CloudFront Domain created at: %s", cfDomain)
	cfCreated = true
	cfDistID = distID

	// if we want to simulate an error to trigger cleanup
	//if true {
	//	err = fmt.Errorf("simulated post-CF creation failure")
	//	return
	//}

	r53Client := route53.NewFromConfig(cfg)

	zoneID, err := getHostedZoneIDByName(ctx, r53Client, "lawangin.io")
	if err != nil {
		err = fmt.Errorf("Unable to get hosted zone ID: %w", err)
		return
	}
	log.Printf("Found hosted zone ID: %s", zoneID)

	err = createSubdomainAliasRecord(
		ctx,
		r53Client,
		zoneID, // ← Replace with your actual hosted zone ID for lawangin.io
		projectName,
		domain,
		cfDomain, // ← Replace with your real CloudFront domain
	)
	if err != nil {
		err = fmt.Errorf("Route 53 record creation failed: %w", err)
		return
	}
	log.Println("Route 53 record successfully created")

}

func uploadFile(s3Client *s3.Client, bucket, key, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Guess content type
	contentType := mime.TypeByExtension(filepath.Ext(filePath))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	_, err = s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        file,
		ContentType: aws.String(contentType),
	})
	return err
}

func uploadFolder(s3Client *s3.Client, bucket, localPath, s3Prefix string) error {
	return filepath.Walk(localPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// Construct S3 key relative to base folder
		relPath, err := filepath.Rel(localPath, path)
		if err != nil {
			return err
		}
		key := filepath.Join(s3Prefix, relPath)

		return uploadFile(s3Client, bucket, key, path)
	})
}

func createCloudFrontDistribution(
	ctx context.Context,
	cfClient *cloudfront.Client,
	subdomain string,
	s3WebsiteEndpoint string,
	sslCertARN string,
) (domain string, distID string, err error) {

	input := &cloudfront.CreateDistributionInput{
		DistributionConfig: &cftypes.DistributionConfig{
			Enabled:         aws.Bool(true),
			CallerReference: aws.String(fmt.Sprintf("lawangin-%d", time.Now().UnixNano())),

			Origins: &cftypes.Origins{
				Items: []cftypes.Origin{
					{
						Id:         aws.String("S3-origin"),
						DomainName: aws.String(s3WebsiteEndpoint),
						CustomOriginConfig: &cftypes.CustomOriginConfig{
							HTTPPort:             aws.Int32(80),
							HTTPSPort:            aws.Int32(443),
							OriginProtocolPolicy: cftypes.OriginProtocolPolicyHttpOnly,
							OriginSslProtocols: &cftypes.OriginSslProtocols{
								Items:    []cftypes.SslProtocol{cftypes.SslProtocolTLSv12},
								Quantity: aws.Int32(1),
							},
							OriginReadTimeout:      aws.Int32(30),
							OriginKeepaliveTimeout: aws.Int32(5),
						},
					},
				},
				Quantity: aws.Int32(1),
			},

			DefaultCacheBehavior: &cftypes.DefaultCacheBehavior{
				TargetOriginId:       aws.String("S3-origin"),
				ViewerProtocolPolicy: cftypes.ViewerProtocolPolicyRedirectToHttps,
				AllowedMethods: &cftypes.AllowedMethods{
					Items:    []cftypes.Method{cftypes.MethodGet, cftypes.MethodHead},
					Quantity: aws.Int32(2),
				},
				CachePolicyId: aws.String("658327ea-f89d-4fab-a63d-7e88639e58f6"),
				Compress:      aws.Bool(true),
			},

			ViewerCertificate: &cftypes.ViewerCertificate{
				ACMCertificateArn:      aws.String(sslCertARN),
				SSLSupportMethod:       cftypes.SSLSupportMethodSniOnly,
				MinimumProtocolVersion: cftypes.MinimumProtocolVersionTLSv122021,
			},

			Aliases: &cftypes.Aliases{
				Quantity: aws.Int32(1),
				Items:    []string{subdomain},
			},

			Comment: aws.String("CloudFront distribution for " + subdomain),
		},
	}

	output, err := cfClient.CreateDistribution(ctx, input)
	if err != nil {
		return "", "", fmt.Errorf("failed to create CloudFront distribution: %w", err)
	}

	return aws.ToString(output.Distribution.DomainName), aws.ToString(output.Distribution.Id), nil
}

func createSubdomainAliasRecord(
	ctx context.Context,
	r53Client *route53.Client,
	hostedZoneID string,
	subdomain string,
	domain string,
	cloudfrontDomain string,
) error {

	// Build the record input
	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(hostedZoneID),
		ChangeBatch: &r53types.ChangeBatch{
			Changes: []r53types.Change{
				{
					Action: r53types.ChangeActionUpsert,
					ResourceRecordSet: &r53types.ResourceRecordSet{
						Name: aws.String(subdomain + domain + "."),
						Type: r53types.RRTypeA,
						AliasTarget: &r53types.AliasTarget{
							DNSName:              aws.String(cloudfrontDomain + "."), // must end in dot
							EvaluateTargetHealth: false,
							HostedZoneId:         aws.String("Z2FDTNDATAQYW2"), // CloudFront fixed HostedZoneId
						},
					},
				},
			},
			Comment: aws.String("Alias for CloudFront static site"),
		},
	}

	// Execute the change
	_, err := r53Client.ChangeResourceRecordSets(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create alias record: %w", err)
	}

	log.Printf("Subdomain record created for %s.%s\n", subdomain, domain)
	return nil
}

func getHostedZoneIDByName(ctx context.Context, r53Client *route53.Client, domainName string) (string, error) {
	// Ensure domain name ends with a dot (AWS format)
	if !strings.HasSuffix(domainName, ".") {
		domainName += "."
	}

	// List all hosted zones
	output, err := r53Client.ListHostedZones(ctx, &route53.ListHostedZonesInput{})
	if err != nil {
		return "", fmt.Errorf("failed to list hosted zones: %w", err)
	}

	// Search for a zone that matches the domain
	for _, zone := range output.HostedZones {
		if aws.ToString(zone.Name) == domainName {
			// HostedZone.Id is in the format "/hostedzone/ID", so we extract just the ID
			parts := strings.Split(aws.ToString(zone.Id), "/")
			return parts[len(parts)-1], nil
		}
	}

	return "", fmt.Errorf("hosted zone for domain %s not found", domainName)
}

func deleteS3Bucket(ctx context.Context, s3Client *s3.Client, bucketName string) {
	log.Println("Rolling back: deleting S3 bucket:", bucketName)

	// Delete all objects first (required)
	objList, _ := s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucketName)})
	for _, obj := range objList.Contents {
		_, _ = s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(bucketName),
			Key:    obj.Key,
		})
	}
	_, _ = s3Client.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(bucketName)})
	log.Println("S3 bucket deleted.")
}

func deleteCloudFrontDistribution(ctx context.Context, cfClient *cloudfront.Client, distID string) {
	log.Println("Rolling back: deleting CloudFront distribution:", distID)

	// First disable the distribution
	getOut, _ := cfClient.GetDistributionConfig(ctx, &cloudfront.GetDistributionConfigInput{
		Id: aws.String(distID),
	})

	getOut.DistributionConfig.Enabled = aws.Bool(false)

	_, _ = cfClient.UpdateDistribution(ctx, &cloudfront.UpdateDistributionInput{
		Id:                 aws.String(distID),
		IfMatch:            aws.String(*getOut.ETag),
		DistributionConfig: getOut.DistributionConfig,
	})

	// Wait a few seconds to allow disable to propagate (CloudFront takes time)
	time.Sleep(20 * time.Second)

	// Then delete
	_, _ = cfClient.DeleteDistribution(ctx, &cloudfront.DeleteDistributionInput{
		Id:      aws.String(distID),
		IfMatch: aws.String(*getOut.ETag),
	})

	log.Println("CloudFront distribution deleted.")
}

func folderSize(path string) (int64, error) {
	var total int64 = 0
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}
