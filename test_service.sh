#!/bin/bash

# Simple Status List Service Test Script
# Usage: ./test_service.sh

API_KEY="local-dev-key"
BASE_URL="http://localhost:8080"

echo "============================================"
echo "  Status List Service Test Script"
echo "============================================"
echo

# Check if service is running
echo "🔍 Checking if service is running..."
if curl -s "$BASE_URL/health" > /dev/null; then
    echo "✅ Service is running"
    curl -s "$BASE_URL/health" | echo "   Health check: $(cat)"
else
    echo "❌ Service is not running. Please start it first:"
    echo "   go run main.go"
    exit 1
fi

echo

# Test 1: Take an index
echo "📝 Test 1: Taking a new index..."
RESPONSE=$(curl -s -X POST "$BASE_URL/token_status_list/take" \
  -H "X-Api-Key: $API_KEY" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "doctype=eu.europa.ec.eudi.pid.1" \
  -d "country=LV" \
  -d "expiry_date=2025-12-31")

if [[ $? -eq 0 ]] && [[ "$RESPONSE" == *"status_list"* ]]; then
    echo "✅ Successfully took an index"
    
    # Extract URI and index using basic text manipulation
    STATUS_URI=$(echo "$RESPONSE" | grep -o '"uri":"[^"]*"' | head -1 | cut -d'"' -f4)
    INDEX=$(echo "$RESPONSE" | grep -o '"idx":[0-9]*' | cut -d':' -f2)
    
    echo "   URI: $STATUS_URI"
    echo "   Index: $INDEX"
    
    if [[ -n "$STATUS_URI" ]] && [[ -n "$INDEX" ]]; then
        echo

        # Test 2: Check initial status
        echo "🔍 Test 2: Checking initial status..."
        ENCODED_URI=$(echo "$STATUS_URI" | sed 's/:/%3A/g; s|/|%2F|g')
        INITIAL_STATUS=$(curl -s "$BASE_URL/token_status_list/get?uri=$ENCODED_URI&idx=$INDEX")

        if [[ "$INITIAL_STATUS" == "0" ]]; then
            echo "✅ Initial status is 0 (valid)"
            echo

            # Test 2b: RFC endpoint - fetch status list JWT
            echo "📦 Test 2b: Fetching status list JWT from RFC endpoint..."
            # Extract /token_status_list/{country}/{doctype}/{id} from URI
            RFC_PATH=$(echo "$STATUS_URI" | sed -E 's|^https?://[^/]+||')

            JWT_RESPONSE=$(curl -s -D - -H "Accept: application/statuslist+jwt" "$BASE_URL$RFC_PATH")
            # echo "--- RFC endpoint raw response ---"
            # echo "$JWT_RESPONSE"
            # echo "--- end of response ---"
            JWT_CONTENT_TYPE=$(echo "$JWT_RESPONSE" | grep -i '^Content-Type:' | tr -d '\r' | awk '{print $2}')
            JWT_BODY=$(echo "$JWT_RESPONSE" | awk '/^\r?$/{flag=1;next}flag')

            if [[ "$JWT_CONTENT_TYPE" == "application/statuslist+jwt" ]] && [[ -n "$JWT_BODY" ]]; then
                echo "✅ RFC endpoint returned JWT with correct Content-Type"
            else
                echo "❌ RFC endpoint failed. Content-Type: $JWT_CONTENT_TYPE, Body: $JWT_BODY"
            fi
            echo

            # Test 2c: RFC endpoint - fetch status list CWT
            echo "📦 Test 2c: Fetching status list CWT from RFC endpoint..."
            CWT_RESPONSE=$(curl -s -D - -H "Accept: application/statuslist+cwt" "$BASE_URL$RFC_PATH")
            CWT_CONTENT_TYPE=$(echo "$CWT_RESPONSE" | grep -i '^Content-Type:' | tr -d '\r' | awk '{print $2}')
            CWT_BODY=$(echo "$CWT_RESPONSE" | awk '/^\r?$/{flag=1;next}flag')

            if [[ "$CWT_CONTENT_TYPE" == "application/statuslist+cwt" ]] && [[ -n "$CWT_BODY" ]]; then
                echo "✅ RFC endpoint returned CWT with correct Content-Type"
            else
                echo "❌ RFC endpoint failed. Content-Type: $CWT_CONTENT_TYPE, Body: $CWT_BODY"
            fi
            echo

            # Test 3: Revoke the token
            echo "🚫 Test 3: Revoking token..."
            REVOKE_RESPONSE=$(curl -s -X POST "$BASE_URL/token_status_list/set" \
              -H "X-Api-Key: $API_KEY" \
              -H "Content-Type: application/x-www-form-urlencoded" \
              -d "uri=$STATUS_URI" \
              -d "idx=$INDEX" \
              -d "status=1")

            if [[ "$REVOKE_RESPONSE" == *"Status Changed"* ]]; then
                echo "✅ Successfully revoked token"
                echo

                # Test 4: Check status after revocation
                echo "🔍 Test 4: Checking status after revocation..."
                FINAL_STATUS=$(curl -s "$BASE_URL/token_status_list/get?uri=$ENCODED_URI&idx=$INDEX")

                if [[ "$FINAL_STATUS" == "1" ]]; then
                    echo "✅ Status is now 1 (revoked)"
                else
                    echo "❌ Expected status 1, got: $FINAL_STATUS"
                fi
            else
                echo "❌ Failed to revoke token: $REVOKE_RESPONSE"
            fi
        else
            echo "❌ Expected initial status 0, got: $INITIAL_STATUS"
        fi
    else
        echo "❌ Could not extract URI or index from response"
    fi
else
    echo "❌ Failed to take index: $RESPONSE"
fi

echo

# Test 5: Error handling - Invalid API key
echo "🔐 Test 5: Testing invalid API key..."
ERROR_RESPONSE=$(curl -s -X POST "$BASE_URL/token_status_list/take" \
  -H "X-Api-Key: wrong_key" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "doctype=eu.europa.ec.eudi.pid.1" \
  -d "country=LV" \
  -d "expiry_date=2025-12-31")

if [[ "$ERROR_RESPONSE" == *"invalid_api_key"* ]]; then
    echo "✅ Correctly rejected invalid API key"
else
    echo "❌ Should have rejected invalid API key. Got: $ERROR_RESPONSE"
fi

echo

# Test 6: Error handling - Invalid doctype
echo "📄 Test 6: Testing invalid doctype..."
ERROR_RESPONSE=$(curl -s -X POST "$BASE_URL/token_status_list/take" \
  -H "X-Api-Key: $API_KEY" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "doctype=invalid.type" \
  -d "country=LV" \
  -d "expiry_date=2025-12-31")

if [[ "$ERROR_RESPONSE" == *"invalid_doctype"* ]]; then
    echo "✅ Correctly rejected invalid doctype"
else
    echo "❌ Should have rejected invalid doctype. Got: $ERROR_RESPONSE"
fi

echo

# Test 7: CWT (CBOR Web Token) format support
echo "🔐 Test 7: Testing CWT format support..."
if [[ -n "$STATUS_URI" ]]; then
    # Extract path from STATUS_URI for direct endpoint testing
    URI_PATH=$(echo "$STATUS_URI" | sed 's|http://[^/]*||')
    
    # Test 7a: Request JWT format (default)
    echo "  7a: Testing JWT format (application/statuslist+jwt)..."
    JWT_RESPONSE=$(curl -s -H "Accept: application/statuslist+jwt" "$BASE_URL$URI_PATH")
    if [[ $? -eq 0 ]] && [[ -n "$JWT_RESPONSE" ]]; then
        echo "  ✅ JWT format response received (length: ${#JWT_RESPONSE} chars)"
        # Basic JWT validation - should have 3 parts separated by dots
        DOT_COUNT=$(echo "$JWT_RESPONSE" | tr -cd '.' | wc -c)
        if [[ $DOT_COUNT -eq 2 ]]; then
            echo "  ✅ JWT format appears valid (has 3 parts)"
        else
            echo "  ⚠️ JWT format may be invalid (expected 3 parts, found $((DOT_COUNT + 1)))"
        fi
    else
        echo "  ❌ Failed to get JWT format response"
    fi
    
    echo
    
    # Test 7b: Request CWT format
    echo "  7b: Testing CWT format (application/statuslist+cwt)..."
    CWT_RESPONSE=$(curl -s -H "Accept: application/statuslist+cwt" "$BASE_URL$URI_PATH")
    if [[ $? -eq 0 ]] && [[ -n "$CWT_RESPONSE" ]]; then
        echo "  ✅ CWT format response received (length: ${#CWT_RESPONSE} chars)"
        # CWT is binary/base64, so we can't easily validate structure here
        # but we can check it's different from JWT
        if [[ "$CWT_RESPONSE" != "$JWT_RESPONSE" ]]; then
            echo "  ✅ CWT format is different from JWT (as expected)"
        else
            echo "  ⚠️ CWT and JWT responses are identical (unexpected)"
        fi
    else
        echo "  ❌ Failed to get CWT format response"
    fi
    
    echo
    
    # Test 7c: Default format (should be JWT)
    echo "  7c: Testing default format (no Accept header)..."
    DEFAULT_RESPONSE=$(curl -s "$BASE_URL$URI_PATH")
    if [[ $? -eq 0 ]] && [[ -n "$DEFAULT_RESPONSE" ]]; then
        echo "  ✅ Default format response received"
        if [[ "$DEFAULT_RESPONSE" == "$JWT_RESPONSE" ]]; then
            echo "  ✅ Default format matches JWT (as expected)"
        else
            echo "  ⚠️ Default format doesn't match JWT"
        fi
    else
        echo "  ❌ Failed to get default format response"
    fi
    
    echo
    
    # Test 7d: Invalid Accept header
    echo "  7d: Testing invalid Accept header..."
    INVALID_RESPONSE=$(curl -s -w "%{http_code}" -H "Accept: application/xml" "$BASE_URL$URI_PATH")
    HTTP_CODE="${INVALID_RESPONSE: -3}"
    if [[ "$HTTP_CODE" == "406" ]]; then
        echo "  ✅ Correctly returned 406 Not Acceptable for unsupported format"
    else
        echo "  ❌ Should have returned 406 for unsupported format, got: $HTTP_CODE"
    fi
else
    echo "❌ Skipping CWT tests - no valid status URI available"
fi

echo

# Test 8: Content-Type header verification
echo "📋 Test 8: Testing Content-Type headers..."
if [[ -n "$STATUS_URI" ]]; then
    URI_PATH=$(echo "$STATUS_URI" | sed 's|http://[^/]*||')
    
    # Test JWT Content-Type
    JWT_HEADERS=$(curl -s -v -H "Accept: application/statuslist+jwt" "$BASE_URL$URI_PATH" 2>&1 | grep -i "content-type")
    if echo "$JWT_HEADERS" | grep -i "application/statuslist+jwt" > /dev/null; then
        echo "  ✅ JWT response has correct Content-Type header"
    else
        echo "  ❌ JWT response missing or incorrect Content-Type header"
        echo "  Debug: $JWT_HEADERS"
    fi
    
    # Test CWT Content-Type  
    CWT_HEADERS=$(curl -s -v -H "Accept: application/statuslist+cwt" "$BASE_URL$URI_PATH" 2>&1 | grep -i "content-type")
    if echo "$CWT_HEADERS" | grep -i "application/statuslist+cwt" > /dev/null; then
        echo "  ✅ CWT response has correct Content-Type header"
    else
        echo "  ❌ CWT response missing or incorrect Content-Type header"
        echo "  Debug: $CWT_HEADERS"
    fi
else
    echo "❌ Skipping Content-Type tests - no valid status URI available"
fi

echo
echo "🎉 All tests completed!"
echo
echo "💡 Next steps:"
echo "   • Check the Swagger documentation: $BASE_URL/token_status_list/swagger"
echo "   • Run load tests with the examples in TESTING.md"
echo "   • Check log files for detailed information"
echo "   • Verify CWT tokens with CBOR decoder tools"
