package database

import (
	"context"
	"log"

	firebase "firebase.google.com/go/v4"
	"cloud.google.com/go/firestore"
	"google.golang.org/api/option"
)

var Client *firestore.Client
var Ctx = context.Background()

func Init() {
	opt := option.WithCredentialsFile("complaint-app-441b7-firebase-adminsdk-fbsvc-71a31afcfa.json")
	app, err := firebase.NewApp(Ctx, nil, opt)
	if err != nil {
		log.Fatalf("❌ Failed to initialize Firebase: %v", err)
	}

	Client, err = app.Firestore(Ctx)
	if err != nil {
		log.Fatalf("❌ Failed to connect to Firestore: %v", err)
	}

	seedCategories()
	log.Println("✅ Connected to Firestore database.")
}

func seedCategories() {
	cats := []string{"Electricity", "Water", "Road", "Internet", "Others"}

	for i, name := range cats {
		docID := string(rune('1' + i))
		ref := Client.Collection("categories").Doc(docID)
		_, err := ref.Get(Ctx)
		if err != nil {
			// Document doesn't exist, create it
			_, err = ref.Set(Ctx, map[string]interface{}{
				"id":   i + 1,
				"name": name,
			})
			if err != nil {
				log.Printf("⚠️ Failed to seed category %s: %v", name, err)
			}
		}
	}
}
