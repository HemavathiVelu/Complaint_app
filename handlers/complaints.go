package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"antigravity/backend/database"
	"antigravity/backend/middleware"

	"github.com/gorilla/mux"
	"google.golang.org/api/iterator"
	"gopkg.in/gomail.v2"
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

		// Firebase Storage upload
		objectName := fmt.Sprintf("complaints/%d_%s", time.Now().UnixMilli(), header.Filename)
		wc := database.Bucket.Object(objectName).NewWriter(database.Ctx)
		wc.ContentType = header.Header.Get("Content-Type")
		if wc.ContentType == "" {
			wc.ContentType = "image/jpeg" // fallback
		}

		if _, err := io.Copy(wc, file); err != nil {
			log.Printf("⚠️ Failed to copy image to storage: %v", err)
		}
		if err := wc.Close(); err != nil {
			log.Printf("⚠️ Failed to close storage writer: %v", err)
		}

		// Construct public-accessible URL (Note: Requires bucket to have public access or appropriate rules)
		// For simplicity, we use the standard firebasestorage v0 URL
		encodedPath := url.PathEscape(objectName)
		storageURL := fmt.Sprintf("https://firebasestorage.googleapis.com/v0/b/%s/o/%s?alt=media", 
			database.BucketName, 
			encodedPath)
		
		imageURL = &storageURL
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

	// If the admin successfully marks a complaint as "Resolved", trigger the email
	if body.Status != nil && *body.Status == "Resolved" {
		go sendResolvedNotification(id)
	}

	json.NewEncoder(w).Encode(map[string]string{"message": "Complaint updated successfully"})
}

// sendResolvedNotification fetches necessary data and sends an email to the user
func sendResolvedNotification(complaintID string) {
	// 1. Fetch the complaint
	doc, err := database.Client.Collection("complaints").Doc(complaintID).Get(database.Ctx)
	if err != nil {
		log.Printf("⚠️ Failed to fetch complaint %s for email notification: %v", complaintID, err)
		return
	}
	data := doc.Data()
	userID, ok1 := data["user_id"].(string)
	title, ok2 := data["title"].(string)

	if !ok1 || !ok2 {
		log.Printf("⚠️ Complaint %s missing user_id or title", complaintID)
		return
	}

	// 2. Fetch the user's details
	userDoc, err := database.Client.Collection("users").Doc(userID).Get(database.Ctx)
	if err != nil {
		log.Printf("⚠️ Failed to fetch user %s for email notification: %v", userID, err)
		return
	}
	userData := userDoc.Data()
	email, eOk := userData["email"].(string)
	name, nOk := userData["name"].(string)

	if !eOk || email == "" {
		log.Println("⚠️ Cannot send resolved email: User has no email associated")
		return
	}
	if !nOk {
		name = "Citizen"
	}

	// 3. Send the email with gomail
	host := os.Getenv("SMTP_HOST")
	if host == "" {
		host = "smtp.gmail.com"
	}
	portStr := os.Getenv("SMTP_PORT")
	port := 587
	if portStr != "" {
		fmt.Sscanf(portStr, "%d", &port)
	}

	senderEmail := os.Getenv("SMTP_EMAIL")
	senderPass := os.Getenv("SMTP_PASSWORD")

	if senderEmail == "" || senderPass == "" {
		log.Println("⚠️ SMTP_EMAIL or SMTP_PASSWORD not set, skipping email notification")
		return
	}

	m := gomail.NewMessage()
	m.SetHeader("From", senderEmail)
	m.SetHeader("To", email)
	m.SetHeader("Subject", "Complaint Resolved: "+title)

	body := fmt.Sprintf("Hello %s,\n\nWe are pleased to inform you that your complaint titled '%s' has been marked as Resolved.\n\nThank you for reaching out to us.\n", name, title)
	m.SetBody("text/plain", body)

	d := gomail.NewDialer(host, port, senderEmail, senderPass)

	if err := d.DialAndSend(m); err != nil {
		log.Printf("⚠️ Failed to send resolved email to %s: %v", email, err)
	} else {
		log.Printf("✅ Resolved email sent successfully to %s", email)
	}
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
