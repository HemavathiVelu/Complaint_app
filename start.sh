#!/bin/bash
# Start the Go backend server
export JWT_SECRET=supersecretkey_for_mini_project
export PORT=5000

echo "🚀 Starting Go backend on port $PORT..."
cd "$(dirname "$0")"
go run .
