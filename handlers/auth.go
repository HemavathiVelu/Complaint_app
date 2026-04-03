package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"antigravity/backend/database"
	"antigravity/backend/middleware"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/api/iterator"
	"gopkg.in/gomail.v2"
	"math/rand"
	"fmt"
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

type VerifyOTPRequest struct {
	Email string `json:"email"`
	OTP   string `json:"otp"`
}

func sendEmail(to string, subject string, body string) error {
	smtpHost := os.Getenv("SMTP_HOST")
	smtpPort := 587 // default
	smtpUser := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASS")

	if smtpHost == "" || smtpUser == "" || smtpPass == "" {
		log.Printf("📧 SIMULATED EMAIL to %s: [%s] %s", to, subject, body)
		return nil
	}

	m := gomail.NewMessage()
	m.SetHeader("From", smtpUser)
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", body)

	d := gomail.NewDialer(smtpHost, smtpPort, smtpUser, smtpPass)
	return d.DialAndSend(m)
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
	email := data["email"].(string)
	hashedPwd := data["password"].(string)

	if err := bcrypt.CompareHashAndPassword([]byte(hashedPwd), []byte(req.Password)); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid password"})
		return
	}

	// Generate 6-digit OTP
	otp := fmt.Sprintf("%06d", rand.Intn(1000000))
	expiry := time.Now().Add(5 * time.Minute)

	// Store OTP in Firestore
	_, err = database.Client.Collection("otps").Doc(email).Set(database.Ctx, map[string]interface{}{
		"otp":     otp,
		"expires": expiry,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to store OTP"})
		return
	}

	// Send OTP Email
	emailBody := fmt.Sprintf("<h2>SOLVEX Login Verification</h2><p>Your verification code is: <b>%s</b></p><p>This code will expire in 5 minutes.</p>", otp)
	if err := sendEmail(email, "Your SOLVEX OTP Code", emailBody); err != nil {
		log.Printf("⚠️ Failed to send email: %v", err)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"requires_otp": true,
		"email":        email,
		"message":      "OTP sent to your email",
	})
}

func VerifyOTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req VerifyOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	// Get OTP from Firestore
	doc, err := database.Client.Collection("otps").Doc(req.Email).Get(database.Ctx)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "OTP not found or expired"})
		return
	}

	data := doc.Data()
	storedOTP := data["otp"].(string)
	expiry := data["expires"].(time.Time)

	if time.Now().After(expiry) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "OTP has expired"})
		return
	}

	if storedOTP != req.OTP {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid OTP"})
		return
	}

	// Delete OTP after successful verification
	database.Client.Collection("otps").Doc(req.Email).Delete(database.Ctx)

	// OTP is correct, find user again to generate token
	iter := database.Client.Collection("users").Where("email", "==", req.Email).Documents(database.Ctx)
	defer iter.Stop()
	userDoc, err := iter.Next()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "User not found during verification"})
		return
	}

	userData := userDoc.Data()
	docID := userDoc.Ref.ID
	name := userData["name"].(string)
	email := userData["email"].(string)
	role := userData["role"].(string)

	// Send "Process Completed" Email
	completionBody := "<h2>SOLVEX - Login Successful</h2><p>Hello, your login process is completed successfully.</p><p>Welcome back to SOLVEX!</p>"
	sendEmail(email, "Login Successful - SOLVEX", completionBody)

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "supersecretkey_for_mini_project"
	}

	claims := middleware.Claims{
		ID:   0,
		Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}
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
