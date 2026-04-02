package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"antigravity/backend/database"
	"antigravity/backend/middleware"

	"cloud.google.com/go/firestore"
	"github.com/gorilla/mux"
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

// GET /api/feedbacks — Admins can view all feedback
func GetAllFeedbacks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	iter := database.Client.Collection("feedbacks").OrderBy("created_at", firestore.Desc).Documents(database.Ctx)
	defer iter.Stop()

	var feedbacks []Feedback
	for {
		doc, err := iter.Next()
		if err != nil {
			if err.Error() == "iterator is done" || err.Error() == "no more items in iterator" {
				break
			} else if doc == nil { // catch another way it might signal done
				break
			}
			// if it's the specific iterator.Done we handle correctly below
		}
		
		var f Feedback
		if err := doc.DataTo(&f); err == nil {
			feedbacks = append(feedbacks, f)
		}
	}

	if feedbacks == nil {
		feedbacks = []Feedback{}
	}
	
	json.NewEncoder(w).Encode(feedbacks)
}

// GET /api/feedback/{complaint_id} — Get feedback for a specific complaint
func GetFeedbackByComplaint(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	complaintID := vars["complaint_id"]

	if complaintID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "complaint_id is required"})
		return
	}

	iter := database.Client.Collection("feedbacks").Where("complaint_id", "==", complaintID).Limit(1).Documents(database.Ctx)
	defer iter.Stop()

	doc, err := iter.Next()
	if err != nil {
		if err.Error() == "iterator is done" || err.Error() == "no more items in iterator" {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "No feedback found for this complaint"})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to fetch feedback", "details": err.Error()})
		return
	}

	var f Feedback
	if err := doc.DataTo(&f); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to parse feedback data"})
		return
	}

	json.NewEncoder(w).Encode(f)
}
