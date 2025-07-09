package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"mime"
	"os"
	"path/filepath"
)

func main() {
	bucketName := "test-bucket-lawanginkhan-5"

	// Load the Shared AWS Configuration (~/.aws/config)
	log.Println("Loading configuration...")
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion("us-east-1"))
	if err != nil {
		log.Fatalf("failed to load configuration, %v", err)
	}

	// Create an Amazon S3 service client
	log.Println("Creating s3 client...")
	client := s3.NewFromConfig(cfg)

	// Get the first page of results for ListObjectsV2 for a bucket
	bucketsList, err := client.ListBuckets(context.TODO(), &s3.ListBucketsInput{})
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
		_, err := client.CreateBucket(context.TODO(), &s3.CreateBucketInput{
			Bucket: aws.String(bucketName),
		})
		if err != nil {
			log.Fatal("Bucket creation failed:", err)
		}
		log.Println("Bucket created:", bucketName)

	} else {
		log.Println("Bucket already exists:", bucketName)
	}

	// enable s3 bucket to utilize static web hosting
	_, err = client.PutBucketWebsite(context.TODO(), &s3.PutBucketWebsiteInput{
		Bucket: aws.String(bucketName),
		WebsiteConfiguration: &types.WebsiteConfiguration{
			IndexDocument: &types.IndexDocument{Suffix: aws.String("index.html")},
			ErrorDocument: &types.ErrorDocument{Key: aws.String("index.html")},
		},
	})
	if err != nil {
		log.Fatal("Failed to enable static website:", err)
	}

	// remove blocked access to bucket
	_, err = client.PutPublicAccessBlock(context.TODO(), &s3.PutPublicAccessBlockInput{
		Bucket: aws.String(bucketName),
		PublicAccessBlockConfiguration: &types.PublicAccessBlockConfiguration{
			BlockPublicAcls:       aws.Bool(false),
			IgnorePublicAcls:      aws.Bool(false),
			BlockPublicPolicy:     aws.Bool(false),
			RestrictPublicBuckets: aws.Bool(false),
		},
	})
	if err != nil {
		log.Fatal("Failed to enable public policy:", err)
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
		log.Fatal("Failed to enable bucket policy:", err)
	}

	err = uploadFolder(client, bucketName, "./build", "")
	if err != nil {
		log.Fatalf("Failed to upload folder: %v", err)
	}

	log.Printf("Site available at: http://%s.s3-website-us-east-1.amazonaws.com\n", bucketName)
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
