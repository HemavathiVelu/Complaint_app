package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"

	"antigravity/backend/database"
	"antigravity/backend/handlers"
	"antigravity/backend/middleware"
)

func main() {
	// Initialize DB
	database.Init()

	r := mux.NewRouter()

	// CORS middleware (applies to all routes)
	r.Use(middleware.CORS)

	// Static files
	r.PathPrefix("/uploads/").Handler(http.StripPrefix("/uploads/", http.FileServer(http.Dir("./uploads"))))

	// Auth routes
	r.HandleFunc("/api/auth/register", handlers.Register).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/auth/login", handlers.Login).Methods("POST", "OPTIONS")

	// Category routes
	r.HandleFunc("/api/categories", handlers.GetCategories).Methods("GET", "OPTIONS")

	// ── Protected routes: register OPTIONS separately (bypasses Auth middleware) ──
	// Complaints - OPTIONS preflight (no auth needed)
	r.HandleFunc("/api/complaints", middleware.PreflightHandler).Methods("OPTIONS")
	r.HandleFunc("/api/complaints/me", middleware.PreflightHandler).Methods("OPTIONS")
	r.HandleFunc("/api/complaints/{id}", middleware.PreflightHandler).Methods("OPTIONS")
	r.HandleFunc("/api/feedback", middleware.PreflightHandler).Methods("OPTIONS")
	r.HandleFunc("/api/feedback/{complaint_id}", middleware.PreflightHandler).Methods("OPTIONS")

	// Complaints - actual protected routes
	r.Handle("/api/complaints", middleware.Auth(http.HandlerFunc(handlers.CreateComplaint))).Methods("POST")
	r.Handle("/api/complaints/me", middleware.Auth(http.HandlerFunc(handlers.GetMyComplaints))).Methods("GET")
	r.Handle("/api/complaints", middleware.Auth(middleware.Admin(http.HandlerFunc(handlers.GetAllComplaints)))).Methods("GET")
	r.Handle("/api/complaints/{id}", middleware.Auth(middleware.Admin(http.HandlerFunc(handlers.UpdateComplaint)))).Methods("PUT")

	// Feedback
	r.Handle("/api/feedback", middleware.Auth(http.HandlerFunc(handlers.SubmitFeedback))).Methods("POST")
	r.Handle("/api/feedback/{complaint_id}", middleware.Auth(http.HandlerFunc(handlers.GetFeedbackByComplaint))).Methods("GET")
	r.Handle("/api/feedbacks", middleware.Auth(http.HandlerFunc(handlers.GetAllFeedbacks))).Methods("GET")

	port := os.Getenv("PORT")
	if port == "" {
		port = "5000"
	}

	log.Printf("🚀 Go server running on port %s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
	}
}

