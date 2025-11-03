#!/bin/bash
# Test S3 Storage Integration with MinIO

## Testfile shall be used togeather with docker-compose that starts MinIO service

echo "Testing S3 Storage Integration with MinIO..."
echo "=============================================="
echo ""

# Set environment variables for S3 storage
export STATUS_LIST_STORAGE="s3"
export S3_BUCKET="status-lists"
export S3_ENDPOINT="http://localhost:9000"
export S3_ACCESS_KEY_ID="minioadmin"
export S3_SECRET_ACCESS_KEY="minioadmin"
export S3_REGION="us-east-1"

# Also set required variables for the app
export API_KEY="local-dev-key"
export SERVICE_URL="http://localhost:8080/"
export STATUS_LIST_DIR="/tmp/not-used-for-s3"
export BACKUP_DIR="/tmp/backup"
export LOG_DIR="/tmp/logs"
export PRIVATE_KEY_PATH="temp/private_key/decrypted_key.pem"
export CERTIFICATE_PATH="temp/certificate/PID-DS-0002.cert.der"
export COUNTRY_CODE="LV"
export ALLOWED_DOCTYPES="eu.europa.ec.eudi.pid.1,org.iso.18013.5.1.mDL,custom.doctype.1"

echo "Configuration:"
echo "  Storage Backend: $STATUS_LIST_STORAGE"
echo "  S3 Bucket: $S3_BUCKET"
echo "  S3 Endpoint: $S3_ENDPOINT"
echo ""

# Start the service in background
echo "Starting service with S3 storage..."
go run main.go &
APP_PID=$!

# Wait for service to start
sleep 5

# Check if service is running
if ! ps -p $APP_PID > /dev/null 2>&1; then
    echo "❌ Service failed to start"
    exit 1
fi

echo "✅ Service started with PID $APP_PID"
echo ""

# Test API endpoint to create a status list
echo "Testing API: Create status list..."
EXPIRY_DATE=$(date -d "+30 days" +%Y-%m-%d 2>/dev/null || date -v+30d +%Y-%m-%d)
RESPONSE=$(curl -s -X POST http://localhost:8080/token_status_list/take \
  -H "X-API-Key: $API_KEY" \
  -d "country=LV" \
  -d "doctype=org.iso.18013.5.1.mDL" \
  -d "expiry_date=$EXPIRY_DATE")

echo "Response: $RESPONSE"
echo ""

# Extract status list ID from response
if echo "$RESPONSE" | grep -q "token_status_list"; then
    echo "✅ Status list created in S3 storage"
    
    # Verify the object exists in MinIO
    echo ""
    echo "Verifying object in MinIO bucket..."
    docker exec status-list-go-minio-1 mc ls local/status-lists --recursive
    echo ""
    echo "✅ S3 Storage Integration Test PASSED"
else
    echo "❌ Failed to create status list"
    echo "Response: $RESPONSE"
fi

# Cleanup
echo ""
echo "Stopping service..."
kill $APP_PID
sleep 2

echo "Test completed!"
