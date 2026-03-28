package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"antigravity/backend/database"
	"antigravity/backend/middleware"

	"github.com/gorilla/mux"
	"google.golang.org/api/iterator"
)

type Complaint struct {
	ID                 string  `json:"id"`
	Title              string  `json:"title"`
	Description        string  `json:"description"`
	CategoryID         int     `json:"category_id"`
	Location           string  `json:"location"`
	UserID             string  `json:"user_id"`
	Status             string  `json:"status"`
	ImageURL           *string `json:"image_url"`
	Remarks            *string `json:"remarks"`
	AssignedDepartment *string `json:"assigned_department"`
	CreatedAt          string  `json:"created_at"`
	CategoryName       *string `json:"category_name,omitempty"`
	UserName           *string `json:"user_name,omitempty"`
}

// POST /api/complaints — citizen creates a complaint (multipart form)
func CreateComplaint(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	claims := r.Context().Value(middleware.UserKey).(*middleware.Claims)

	r.ParseMultipartForm(10 << 20) // 10 MB max

	title := r.FormValue("title")
	description := r.FormValue("description")
	categoryIDStr := r.FormValue("category_id")
	location := r.FormValue("location")

	categoryID := 0
	fmt.Sscanf(categoryIDStr, "%d", &categoryID)

	var imageURL *string
	file, header, err := r.FormFile("image")
	if err == nil {
		defer file.Close()
		os.MkdirAll("./uploads", os.ModePerm)
		filename := fmt.Sprintf("%d%s", time.Now().UnixMilli(), filepath.Ext(header.Filename))
		outPath := filepath.Join("./uploads", filename)
		out, err := os.Create(outPath)
		if err == nil {
			defer out.Close()
			io.Copy(out, file)
			url := "/uploads/" + filename
			imageURL = &url
		}
	}

	ref, _, err := database.Client.Collection("complaints").Add(database.Ctx, map[string]interface{}{
		"title":               title,
		"description":         description,
		"category_id":         categoryID,
		"location":            location,
		"user_id":             claims.RegisteredClaims.Subject,
		"status":              "Pending",
		"image_url":           imageURL,
		"remarks":             nil,
		"assigned_department": nil,
		"created_at":          time.Now().Format(time.RFC3339),
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":     "Complaint registered",
		"complaintId": ref.ID,
	})
}

// GET /api/complaints/me — citizen views own complaints
func GetMyComplaints(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	claims := r.Context().Value(middleware.UserKey).(*middleware.Claims)
	userID := claims.RegisteredClaims.Subject

	iter := database.Client.Collection("complaints").Where("user_id", "==", userID).Documents(database.Ctx)
	defer iter.Stop()

	var complaints []Complaint
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
		c := docToComplaint(doc.Ref.ID, doc.Data(), false)
		complaints = append(complaints, c)
	}

	if complaints == nil {
		complaints = []Complaint{}
	}
	json.NewEncoder(w).Encode(complaints)
}

// GET /api/complaints — admin views all complaints with optional filters
func GetAllComplaints(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	categoryIDStr := r.URL.Query().Get("category_id")
	status := r.URL.Query().Get("status")

	query := database.Client.Collection("complaints").Query
	if categoryIDStr != "" {
		catID := 0
		fmt.Sscanf(categoryIDStr, "%d", &catID)
		query = query.Where("category_id", "==", catID)
	}
	if status != "" {
		query = query.Where("status", "==", status)
	}

	iter := query.Documents(database.Ctx)
	defer iter.Stop()

	// Fetch all users for name lookup
	userNames := map[string]string{}
	userIter := database.Client.Collection("users").Documents(database.Ctx)
	for {
		ud, err := userIter.Next()
		if err == iterator.Done {
			break
		}
		if err == nil {
			userNames[ud.Ref.ID] = ud.Data()["name"].(string)
		}
	}
	userIter.Stop()

	var complaints []Complaint
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
		c := docToComplaint(doc.Ref.ID, doc.Data(), true)
		if name, ok := userNames[c.UserID]; ok {
			c.UserName = &name
		}
		complaints = append(complaints, c)
	}

	if complaints == nil {
		complaints = []Complaint{}
	}
	json.NewEncoder(w).Encode(complaints)
}

// PUT /api/complaints/:id — admin updates complaint status
func UpdateComplaint(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	id := vars["id"]

	var body struct {
		Status             *string `json:"status"`
		Remarks            *string `json:"remarks"`
		AssignedDepartment *string `json:"assigned_department"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	updates := map[string]interface{}{}
	if body.Status != nil {
		updates["status"] = *body.Status
	}
	if body.Remarks != nil {
		updates["remarks"] = *body.Remarks
	}
	if body.AssignedDepartment != nil {
		updates["assigned_department"] = *body.AssignedDepartment
	}

	if len(updates) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "No fields to update"})
		return
	}

	_, err := database.Client.Collection("complaints").Doc(id).Set(database.Ctx, updates, firestoreMerge())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"message": "Complaint updated successfully"})
}

// Helper: convert Firestore doc to Complaint struct
func docToComplaint(id string, data map[string]interface{}, withUser bool) Complaint {
	c := Complaint{ID: id}
	if v, ok := data["title"].(string); ok {
		c.Title = v
	}
	if v, ok := data["description"].(string); ok {
		c.Description = v
	}
	if v, ok := data["category_id"].(int64); ok {
		c.CategoryID = int(v)
	}
	if v, ok := data["location"].(string); ok {
		c.Location = v
	}
	if v, ok := data["user_id"].(string); ok {
		c.UserID = v
	}
	if v, ok := data["status"].(string); ok {
		c.Status = v
	}
	if v, ok := data["image_url"].(string); ok {
		c.ImageURL = &v
	}
	if v, ok := data["remarks"].(string); ok {
		c.Remarks = &v
	}
	if v, ok := data["assigned_department"].(string); ok {
		c.AssignedDepartment = &v
	}
	if v, ok := data["created_at"].(string); ok {
		c.CreatedAt = v
	}
	return c
}
