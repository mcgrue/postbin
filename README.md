# Request Logger

A simple HTTP request bin service that captures and stores incoming HTTP requests.

## Getting Started

```bash
go run main.go
```

The server will start on port 8080.

## Manual Testing with curl

### 1. Create a new bin
```bash
# Create a bin and store the bin_id
BIN_ID=$(curl -s -X POST http://localhost:8080/api/bin | jq -r .binId)
echo "Bin ID: $BIN_ID"
```

### 2. Send requests to the bin
```bash
# Send a GET request
curl -X GET "http://localhost:8080/$BIN_ID?param1=value1"

# Send a POST request with JSON body
curl -X POST "http://localhost:8080/$BIN_ID" \
  -H "Content-Type: application/json" \
  -d '{"hello":"world"}'

# Send a POST request with form data
curl -X POST "http://localhost:8080/$BIN_ID" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "field1=value1&field2=value2"

# Send request with custom headers
curl -X POST "http://localhost:8080/$BIN_ID" \
  -H "Custom-Header: custom-value" \
  -H "Another-Header: another-value" \
  -d "some data"
```

### 3. Retrieve bin information
```bash
# Get bin info and request count
curl -s "http://localhost:8080/api/bin/$BIN_ID" | jq .
```

### 4. Retrieve and remove the oldest request (FIFO)
```bash
# Shift (retrieve and remove) the oldest request
curl -s "http://localhost:8080/api/bin/$BIN_ID/req/shift" | jq .
```

### 5. Delete the bin
```bash
# Delete the bin and all its requests
curl -X DELETE "http://localhost:8080/api/bin/$BIN_ID"
```

### Complete Test Sequence
```bash
# Create a new bin
BIN_ID=$(curl -s -X POST http://localhost:8080/api/bin | jq -r .binId)
echo "Created bin: $BIN_ID"

# Send some test requests
curl -X POST "http://localhost:8080/$BIN_ID" \
  -H "Content-Type: application/json" \
  -d '{"test":"data1"}'

curl -X POST "http://localhost:8080/$BIN_ID" \
  -H "Content-Type: application/json" \
  -d '{"test":"data2"}'

# Check bin info and request count
curl -s "http://localhost:8080/api/bin/$BIN_ID" | jq .

# Retrieve the first request (FIFO)
curl -s "http://localhost:8080/api/bin/$BIN_ID/req/shift" | jq .

# Check updated count
curl -s "http://localhost:8080/api/bin/$BIN_ID" | jq .

# Delete the bin
curl -X DELETE "http://localhost:8080/api/bin/$BIN_ID"

# Verify bin is deleted
curl -s "http://localhost:8080/api/bin/$BIN_ID"
```

Note: These examples use `jq` for JSON formatting. Install it with:
- Ubuntu/Debian: `sudo apt-get install jq`
- macOS: `brew install jq`
- Windows: `choco install jq`

If you don't have `jq`, you can omit the `| jq .` parts from the commands.