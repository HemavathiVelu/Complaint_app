package handlers

import (
	"encoding/json"
	"net/http"

	"antigravity/backend/database"
	"google.golang.org/api/iterator"
)

type Category struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func GetCategories(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	iter := database.Client.Collection("categories").Documents(database.Ctx)
	defer iter.Stop()

	var categories []Category
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		data := doc.Data()
		c := Category{}
		if v, ok := data["id"].(int64); ok {
			c.ID = int(v)
		}
		if v, ok := data["name"].(string); ok {
			c.Name = v
		}
		categories = append(categories, c)
	}

	if categories == nil {
		categories = []Category{}
	}
	json.NewEncoder(w).Encode(categories)
}
