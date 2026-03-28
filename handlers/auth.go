package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"antigravity/backend/database"
	"antigravity/backend/middleware"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/api/iterator"
)

type RegisterRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func Register(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	// Check if email already exists
	iter := database.Client.Collection("users").Where("email", "==", req.Email).Documents(database.Ctx)
	defer iter.Stop()
	doc, err := iter.Next()
	if err == nil && doc != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Email already exists"})
		return
	}

	role := "citizen"
	if req.Role == "admin" {
		role = "admin"
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), 10)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Server error"})
		return
	}

	// Add user to Firestore
	ref, _, err := database.Client.Collection("users").Add(database.Ctx, map[string]interface{}{
		"name":     req.Name,
		"email":    req.Email,
		"password": string(hashed),
		"role":     role,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to register user"})
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "User registered successfully",
		"userId":  ref.ID,
	})
}

func Login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	// Find user by email
	iter := database.Client.Collection("users").Where("email", "==", req.Email).Documents(database.Ctx)
	defer iter.Stop()

	doc, err := iter.Next()
	if err == iterator.Done || doc == nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "User not found"})
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Database error"})
		return
	}

	data := doc.Data()
	docID := doc.Ref.ID
	name := data["name"].(string)
	email := data["email"].(string)
	hashedPwd := data["password"].(string)
	role := data["role"].(string)

	if err := bcrypt.CompareHashAndPassword([]byte(hashedPwd), []byte(req.Password)); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid password"})
		return
	}

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "supersecretkey_for_mini_project"
	}

	claims := middleware.Claims{
		ID:   0, // using DocID string now
		Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}

	// Store docID as subject
	claims.RegisteredClaims.Subject = docID

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(secret))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to generate token"})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"token": tokenStr,
		"user":  map[string]interface{}{"id": docID, "name": name, "email": email, "role": role},
	})
}
