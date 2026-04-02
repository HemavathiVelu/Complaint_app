package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"antigravity/backend/database"
	"antigravity/backend/middleware"
)

type Feedback struct {
	ComplaintID  string `json:"complaint_id"`
	PersonName   string `json:"person_name"`
	Subject      string `json:"subject"`
	Category     string `json:"category"`
	IssueType    string `json:"issue_type"`
	Location     string `json:"location"`
	Pincode      string `json:"pincode"`
	State        string `json:"state"`
	Rating       int    `json:"rating"`
	Comment      string `json:"comment"`
	UserID       string `json:"user_id"`
	CreatedAt    string `json:"created_at"`
}

// POST /api/feedback — Users post feedback linked or independent of a complaint
func SubmitFeedback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Get Auth token payload
	claims := r.Context().Value(middleware.UserKey).(*middleware.Claims)
	userID := claims.RegisteredClaims.Subject

	var body Feedback
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON format"})
		return
	}

	body.UserID = userID
	body.CreatedAt = time.Now().Format(time.RFC3339)

	ref, _, err := database.Client.Collection("feedbacks").Add(database.Ctx, map[string]interface{}{
		"complaint_id": body.ComplaintID,
		"person_name":  body.PersonName,
		"subject":      body.Subject,
		"category":     body.Category,
		"issue_type":   body.IssueType,
		"location":     body.Location,
		"pincode":      body.Pincode,
		"state":        body.State,
		"rating":       body.Rating,
		"comment":      body.Comment,
		"user_id":      body.UserID,
		"created_at":   body.CreatedAt,
	})

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to submit feedback", "details": err.Error()})
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":   "Feedback submitted successfully",
		"feedbackId": ref.ID,
	})
}
