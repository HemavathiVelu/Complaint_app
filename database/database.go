package database

import (
	"context"
	"encoding/base64"
	"log"
	"os"

	firebase "firebase.google.com/go/v4"
	"cloud.google.com/go/firestore"
	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

var Client *firestore.Client
var Bucket *storage.BucketHandle
var BucketName = "complaint-app-441b7.appspot.com"
var Ctx = context.Background()

func Init() {
	var opt option.ClientOption

	credsBase64 := os.Getenv("FIREBASE_CREDENTIALS_BASE64")
	if credsBase64 != "" {
		credsJSON, err := base64.StdEncoding.DecodeString(credsBase64)
		if err != nil {
			log.Fatalf("❌ Failed to decode Firebase credentials: %v", err)
		}
		opt = option.WithCredentialsJSON(credsJSON)
	} else {
		opt = option.WithCredentialsFile("complaint-app-441b7-firebase-adminsdk-fbsvc-f907cbe2c3.json")
	}

	config := &firebase.Config{
		StorageBucket: BucketName,
	}

	app, err := firebase.NewApp(Ctx, config, opt)
	if err != nil {
		log.Fatalf("❌ Failed to initialize Firebase: %v", err)
	}

	Client, err = app.Firestore(Ctx)
	if err != nil {
		log.Fatalf("❌ Failed to connect to Firestore: %v", err)
	}

	storageClient, err := app.Storage(Ctx)
	if err != nil {
		log.Fatalf("❌ Failed to connect to Storage: %v", err)
	}

	Bucket, err = storageClient.DefaultBucket()
	if err != nil {
		log.Fatalf("❌ Failed to get default Storage bucket: %v", err)
	}

	seedCategories()
	log.Println("✅ Connected to Firestore & Storage.")
}

func seedCategories() {
	cats := []string{
		"Education and learning",
		"Electricity",
		"Transport and infrastructure",
		"Agriculture",
		"Rural and environment",
		"Water",
		"Road",
		"Street Light",
	}
	for i, name := range cats {
		docID := string(rune('1' + i))
		ref := Client.Collection("categories").Doc(docID)
		_, err := ref.Set(Ctx, map[string]interface{}{
			"id":   i + 1,
			"name": name,
		})
		if err != nil {
			log.Printf("⚠️ Failed to seed category %s: %v", name, err)
		}
	}
}