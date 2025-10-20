# S3 Storage System - API Reference

Complete API reference for the S3-compatible object storage system.

## Table of Contents

- [Overview](#overview)
- [Authentication](#authentication)
- [Standard S3 Operations](#standard-s3-operations)
- [Extended Operations](#extended-operations)
- [Error Responses](#error-responses)
- [Request Headers](#request-headers)
- [Response Headers](#response-headers)
- [Status Codes](#status-codes)

---

## Overview

The S3 Storage System provides a fully S3-compatible RESTful API along with extended operations for advanced features.

**Base URL:** `http://your-server:9000` or `https://your-domain.com`

**API Version:** 1.0.0

**Content-Type:** 
- Requests: `application/octet-stream`, `application/json`, `text/plain`, etc.
- Responses: `application/xml` (S3 operations), `application/json` (extended operations)

---

## Authentication

### Supported Methods

#### 1. HMAC-SHA256 Signature (Recommended)

**Header Format:**
```
Authorization: S3-HMAC-SHA256 AccessKey={access_key},Signature={signature}
Date: {RFC1123_date}
```

**Signature Calculation:**
```
string_to_sign = "{HTTP_METHOD}\n{PATH}\n{DATE}"
signature = base64(HMAC-SHA256(secret_key, string_to_sign))
```

**Example:**
```bash
METHOD="PUT"
PATH="/mybucket/file.txt"
DATE="Mon, 17 Oct 2025 12:00:00 GMT"
STRING_TO_SIGN="$METHOD\n$PATH\n$DATE"
SIGNATURE=$(echo -n "$STRING_TO_SIGN" | openssl dgst -sha256 -hmac "$SECRET_KEY" -binary | base64)

curl -X PUT http://localhost:9000/mybucket/file.txt \
  -H "Authorization: S3-HMAC-SHA256 AccessKey=AKEXAMPLE001,Signature=$SIGNATURE" \
  -H "Date: $DATE" \
  -d "Hello World"
```

#### 2. AWS Signature V4

Compatible with AWS SDK libraries.

**Header Format:**
```
Authorization: AWS4-HMAC-SHA256 Credential={access_key}/{date}/{region}/s3/aws4_request, SignedHeaders={headers}, Signature={signature}
```

**Example (using AWS SDK):**
```python
import boto3

s3 = boto3.client(
    's3',
    endpoint_url='http://localhost:9000',
    aws_access_key_id='AKEXAMPLE001',
    aws_secret_access_key='your-secret-key'
)
```

#### 3. Bearer Token

Simple token-based authentication.

**Header Format:**
```
Authorization: Bearer {token}
```

**Example:**
```bash
curl -X GET http://localhost:9000/mybucket/file.txt \
  -H "Authorization: Bearer your-token-here"
```

#### 4. Presigned URLs

Temporary URLs with embedded authentication.

**URL Format:**
```
http://localhost:9000/{bucket}/{key}?AWSAccessKeyId={access_key}&Signature={signature}&Expires={timestamp}
```

---

## Standard S3 Operations

### PutObject

Upload an object to a bucket.

**Endpoint:** `PUT /{bucket}/{key}`

**Request Headers:**
- `Content-Type` (optional): MIME type of the object
- `Content-Length` (required): Size of the object in bytes
- `Content-MD5` (optional): Base64-encoded MD5 hash for integrity check
- `x-amz-meta-*` (optional): Custom metadata

**Request Body:** Object data (binary or text)

**Response:**
- **Status:** 200 OK
- **Headers:**
  - `ETag`: MD5 hash of the object

**Example:**
```bash
curl -X PUT http://localhost:9000/mybucket/document.pdf \
  -H "Content-Type: application/pdf" \
  -H "Content-MD5: XrY7u+Ae7tCTyyK7j1rNww==" \
  --data-binary @document.pdf

# Response:
# HTTP/1.1 200 OK
# ETag: "5eb63bbbe01eeed093cb22bb8f5acdc3"
```

**Error Codes:**
- `400 BadDigest`: Content-MD5 mismatch
- `403 AccessDenied`: Authentication failed
- `500 InternalError`: Server error

---

### GetObject

Download an object from a bucket.

**Endpoint:** `GET /{bucket}/{key}`

**Request Headers:**
- `Range` (optional): Byte range to retrieve (e.g., `bytes=0-1023`)
- `If-Modified-Since` (optional): Conditional request
- `If-None-Match` (optional): Conditional request based on ETag

**Query Parameters:**
- `versionId` (optional): Specific version to retrieve

**Response:**
- **Status:** 200 OK (full object) or 206 Partial Content (range request)
- **Headers:**
  - `Content-Type`: MIME type of the object
  - `Content-Length`: Size of the response body
  - `ETag`: MD5 hash of the object
  - `Accept-Ranges`: `bytes`
  - `Last-Modified`: Last modification date
  - `Content-Range` (for range requests): Byte range returned

**Response Body:** Object data

**Example:**
```bash
# Full object
curl http://localhost:9000/mybucket/document.pdf \
  -o document.pdf

# Range request (first 1KB)
curl http://localhost:9000/mybucket/document.pdf \
  -H "Range: bytes=0-1023" \
  -o document_partial.pdf

# Specific version
curl http://localhost:9000/mybucket/document.pdf?versionId=v123
```

**Error Codes:**
- `404 NoSuchKey`: Object not found
- `416 InvalidRange`: Requested range not satisfiable
- `403 AccessDenied`: Authentication failed

---

### HeadObject

Retrieve object metadata without downloading the object.

**Endpoint:** `HEAD /{bucket}/{key}`

**Request Headers:**
- `If-Modified-Since` (optional): Conditional request
- `If-None-Match` (optional): Conditional request based on ETag

**Query Parameters:**
- `versionId` (optional): Specific version to query

**Response:**
- **Status:** 200 OK
- **Headers:**
  - `Content-Type`: MIME type of the object
  - `Content-Length`: Size of the object
  - `ETag`: MD5 hash of the object
  - `Last-Modified`: Last modification date
  - `Accept-Ranges`: `bytes`
  - `x-amz-meta-*`: Custom metadata

**Response Body:** Empty

**Example:**
```bash
curl -I http://localhost:9000/mybucket/document.pdf

# Response:
# HTTP/1.1 200 OK
# Content-Type: application/pdf
# Content-Length: 1048576
# ETag: "5eb63bbbe01eeed093cb22bb8f5acdc3"
# Last-Modified: Mon, 17 Oct 2025 12:00:00 GMT
# Accept-Ranges: bytes
```

**Error Codes:**
- `404 NoSuchKey`: Object not found
- `403 AccessDenied`: Authentication failed

---

### DeleteObject

Delete an object from a bucket.

**Endpoint:** `DELETE /{bucket}/{key}`

**Query Parameters:**
- `versionId` (optional): Specific version to delete

**Response:**
- **Status:** 204 No Content
- **Headers:** None

**Response Body:** Empty

**Example:**
```bash
curl -X DELETE http://localhost:9000/mybucket/document.pdf

# Response:
# HTTP/1.1 204 No Content
```

**Error Codes:**
- `403 AccessDenied`: Authentication failed
- `500 InternalError`: Server error

**Note:** Deleting a non-existent object returns 204 (idempotent operation).

---

### ListObjectsV2

List objects in a bucket.

**Endpoint:** `GET /{bucket}?list-type=2`

**Query Parameters:**
- `list-type=2` (required): Use V2 API
- `prefix` (optional): Filter objects by prefix
- `delimiter` (optional): Delimiter for grouping (e.g., `/`)
- `max-keys` (optional): Maximum number of objects to return (1-1000, default: 1000)
- `continuation-token` (optional): Token for pagination
- `start-after` (optional): Start listing after this key

**Response:**
- **Status:** 200 OK
- **Content-Type:** `application/xml`

**Response Body:**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult>
  <Name>mybucket</Name>
  <Prefix>docs/</Prefix>
  <KeyCount>2</KeyCount>
  <MaxKeys>1000</MaxKeys>
  <IsTruncated>false</IsTruncated>
  <Contents>
    <Key>docs/file1.txt</Key>
    <LastModified>2025-10-17T12:00:00.000Z</LastModified>
    <ETag>"abc123"</ETag>
    <Size>1024</Size>
    <StorageClass>STANDARD</StorageClass>
  </Contents>
  <Contents>
    <Key>docs/file2.txt</Key>
    <LastModified>2025-10-17T13:00:00.000Z</LastModified>
    <ETag>"def456"</ETag>
    <Size>2048</Size>
    <StorageClass>STANDARD</StorageClass>
  </Contents>
</ListBucketResult>
```

**Example:**
```bash
# List all objects
curl "http://localhost:9000/mybucket?list-type=2"

# List with prefix
curl "http://localhost:9000/mybucket?list-type=2&prefix=docs/"

# List with pagination
curl "http://localhost:9000/mybucket?list-type=2&max-keys=100"

# Continue from previous page
curl "http://localhost:9000/mybucket?list-type=2&continuation-token=TOKEN"
```

**Error Codes:**
- `400 InvalidArgument`: Invalid parameters
- `403 AccessDenied`: Authentication failed

---

### Multipart Upload

Upload large objects in parts.

#### InitiateMultipartUpload

**Endpoint:** `POST /{bucket}/{key}?uploads`

**Response:**
- **Status:** 200 OK
- **Content-Type:** `application/xml`

**Response Body:**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<InitiateMultipartUploadResult>
  <Bucket>mybucket</Bucket>
  <Key>largefile.bin</Key>
  <UploadId>upload-12345</UploadId>
</InitiateMultipartUploadResult>
```

**Example:**
```bash
UPLOAD_ID=$(curl -X POST "http://localhost:9000/mybucket/largefile.bin?uploads" \
  | grep -oP '(?<=<UploadId>)[^<]+')

echo "Upload ID: $UPLOAD_ID"
```

#### UploadPart

**Endpoint:** `PUT /{bucket}/{key}?partNumber={part_num}&uploadId={upload_id}`

**Request Headers:**
- `Content-Length` (required): Size of the part

**Request Body:** Part data

**Response:**
- **Status:** 200 OK
- **Headers:**
  - `ETag`: MD5 hash of the part (needed for completion)

**Example:**
```bash
ETAG=$(curl -X PUT \
  "http://localhost:9000/mybucket/largefile.bin?partNumber=1&uploadId=$UPLOAD_ID" \
  --data-binary @part1.bin \
  -I | grep -i etag | awk '{print $2}')

echo "Part 1 ETag: $ETAG"
```

#### CompleteMultipartUpload

**Endpoint:** `POST /{bucket}/{key}?uploadId={upload_id}`

**Request Body:**
```xml
<CompleteMultipartUpload>
  <Part>
    <PartNumber>1</PartNumber>
    <ETag>"abc123"</ETag>
  </Part>
  <Part>
    <PartNumber>2</PartNumber>
    <ETag>"def456"</ETag>
  </Part>
</CompleteMultipartUpload>
```

**Response:**
- **Status:** 200 OK
- **Content-Type:** `application/xml`

**Response Body:**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<CompleteMultipartUploadResult>
  <Location>http://localhost:9000/mybucket/largefile.bin</Location>
  <Bucket>mybucket</Bucket>
  <Key>largefile.bin</Key>
  <ETag>"final-etag"</ETag>
</CompleteMultipartUploadResult>
```

**Example:**
```bash
curl -X POST "http://localhost:9000/mybucket/largefile.bin?uploadId=$UPLOAD_ID" \
  -H "Content-Type: application/xml" \
  -d "<CompleteMultipartUpload>
        <Part><PartNumber>1</PartNumber><ETag>$ETAG1</ETag></Part>
        <Part><PartNumber>2</PartNumber><ETag>$ETAG2</ETag></Part>
      </CompleteMultipartUpload>"
```

#### AbortMultipartUpload

**Endpoint:** `DELETE /{bucket}/{key}?uploadId={upload_id}`

**Response:**
- **Status:** 204 No Content

**Example:**
```bash
curl -X DELETE "http://localhost:9000/mybucket/largefile.bin?uploadId=$UPLOAD_ID"
```

---

## Extended Operations

### Object Versioning

Enable versioning for a bucket to maintain object history.

#### Enable Versioning

**Endpoint:** `PUT /{bucket}?versioning`

**Request Body:**
```json
{
  "enabled": true,
  "max_versions": 10,
  "retention_days": 90
}
```

**Response:**
- **Status:** 200 OK
- **Content-Type:** `application/json`

**Response Body:**
```json
{
  "status": "success",
  "message": "Versioning enabled for bucket: mybucket"
}
```

**Example:**
```bash
curl -X PUT http://localhost:9000/mybucket?versioning \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "max_versions": 10,
    "retention_days": 90
  }'
```

#### Get Versioning Status

**Endpoint:** `GET /{bucket}?versioning`

**Response:**
```json
{
  "enabled": true,
  "max_versions": 10,
  "retention_days": 90
}
```

#### List Object Versions

**Endpoint:** `GET /{bucket}/{key}?versions`

**Response:**
```json
{
  "versions": [
    {
      "version_id": "v3",
      "size": 2048,
      "etag": "abc123",
      "last_modified": "2025-10-17T15:00:00Z",
      "is_latest": true
    },
    {
      "version_id": "v2",
      "size": 1024,
      "etag": "def456",
      "last_modified": "2025-10-17T14:00:00Z",
      "is_latest": false
    }
  ]
}
```

#### Restore Version

**Endpoint:** `POST /{bucket}/{key}?restore&versionId={version_id}`

**Response:**
```json
{
  "status": "success",
  "message": "Version v2 restored as latest",
  "new_version_id": "v4"
}
```

---

### Batch Operations

Perform operations on multiple objects in a single request.

**Endpoint:** `POST /batch`

**Request Body:**
```json
{
  "operation": "delete",
  "operations": [
    {"bucket": "mybucket", "key": "file1.txt"},
    {"bucket": "mybucket", "key": "file2.txt"},
    {"bucket": "mybucket", "key": "folder/file3.txt"}
  ],
  "options": {
    "concurrency": 50,
    "dry_run": false,
    "continue_on_error": true
  }
}
```

**Supported Operations:**
- `delete`: Delete multiple objects
- `copy`: Copy objects
- `move`: Move objects
- `restore`: Restore object versions

**Response:**
- **Status:** 200 OK
- **Content-Type:** `application/json`

**Response Body:**
```json
{
  "status": "completed",
  "total": 3,
  "successful": 3,
  "failed": 0,
  "results": [
    {
      "bucket": "mybucket",
      "key": "file1.txt",
      "success": true
    },
    {
      "bucket": "mybucket",
      "key": "file2.txt",
      "success": true
    },
    {
      "bucket": "mybucket",
      "key": "folder/file3.txt",
      "success": true
    }
  ],
  "duration_ms": 1250
}
```

**Example:**
```bash
curl -X POST http://localhost:9000/batch \
  -H "Content-Type: application/json" \
  -d '{
    "operation": "delete",
    "operations": [
      {"bucket": "logs", "key": "2024/01/01/app.log"},
      {"bucket": "logs", "key": "2024/01/02/app.log"}
    ],
    "options": {
      "concurrency": 50
    }
  }'
```

**Error Response:**
```json
{
  "status": "partial",
  "total": 3,
  "successful": 2,
  "failed": 1,
  "results": [
    {"bucket": "mybucket", "key": "file1.txt", "success": true},
    {"bucket": "mybucket", "key": "file2.txt", "success": true},
    {"bucket": "mybucket", "key": "file3.txt", "success": false, "error": "NoSuchKey"}
  ]
}
```

---

### Presigned URLs

Generate temporary URLs for secure object access without sharing credentials.

**Endpoint:** `POST /presign`

**Request Body:**
```json
{
  "bucket": "mybucket",
  "key": "document.pdf",
  "operation": "GET",
  "expires": 3600
}
```

**Operations:**
- `GET`: Download access
- `PUT`: Upload access
- `DELETE`: Delete access

**Response:**
```json
{
  "url": "http://localhost:9000/mybucket/document.pdf?AWSAccessKeyId=AKEXAMPLE&Signature=abc123&Expires=1634473200",
  "expires_at": "2025-10-17T13:00:00Z"
}
```

**Example:**
```bash
# Generate presigned URL
PRESIGNED_URL=$(curl -X POST http://localhost:9000/presign \
  -H "Content-Type: application/json" \
  -d '{
    "bucket": "mybucket",
    "key": "document.pdf",
    "operation": "GET",
    "expires": 3600
  }' | jq -r '.url')

# Use presigned URL (no authentication needed)
curl "$PRESIGNED_URL" -o document.pdf
```

---

### Lifecycle Policies

Configure automatic object expiration based on age.

#### Set Lifecycle Policy

**Endpoint:** `PUT /{bucket}?lifecycle`

**Request Body:**
```json
{
  "rules": [
    {
      "id": "expire-logs",
      "prefix": "logs/",
      "enabled": true,
      "expiration_days": 30
    },
    {
      "id": "expire-temp",
      "prefix": "temp/",
      "enabled": true,
      "expiration_days": 7
    }
  ]
}
```

**Response:**
```json
{
  "status": "success",
  "message": "Lifecycle policy set for bucket: mybucket"
}
```

**Example:**
```bash
curl -X PUT http://localhost:9000/mybucket?lifecycle \
  -H "Content-Type: application/json" \
  -d '{
    "rules": [
      {
        "id": "expire-old-logs",
        "prefix": "logs/",
        "enabled": true,
        "expiration_days": 30
      }
    ]
  }'
```

#### Get Lifecycle Policy

**Endpoint:** `GET /{bucket}?lifecycle`

**Response:**
```json
{
  "rules": [
    {
      "id": "expire-logs",
      "prefix": "logs/",
      "enabled": true,
      "expiration_days": 30
    }
  ]
}
```

#### Delete Lifecycle Policy

**Endpoint:** `DELETE /{bucket}?lifecycle`

**Response:**
```json
{
  "status": "success",
  "message": "Lifecycle policy deleted for bucket: mybucket"
}
```

---

### Concurrent Upload

Upload large files using parallel chunks for faster transfer.

#### Initiate Concurrent Upload

**Endpoint:** `POST /concurrent-upload/initiate`

**Request Body:**
```json
{
  "bucket": "videos",
  "key": "movie.mp4",
  "total_size": 1073741824,
  "chunk_size": 10485760,
  "concurrency": 8
}
```

**Response:**
```json
{
  "upload_id": "cu-12345",
  "total_chunks": 102,
  "upload_urls": [
    "http://localhost:9000/concurrent-upload/chunk?uploadId=cu-12345&chunk=0",
    "http://localhost:9000/concurrent-upload/chunk?uploadId=cu-12345&chunk=1",
    "..."
  ]
}
```

#### Upload Chunk

**Endpoint:** `PUT /concurrent-upload/chunk?uploadId={upload_id}&chunk={chunk_num}`

**Request Body:** Chunk data

**Response:**
```json
{
  "chunk": 0,
  "etag": "abc123",
  "status": "success"
}
```

#### Complete Concurrent Upload

**Endpoint:** `POST /concurrent-upload/complete?uploadId={upload_id}`

**Response:**
```json
{
  "status": "success",
  "bucket": "videos",
  "key": "movie.mp4",
  "etag": "final-etag",
  "size": 1073741824
}
```

---

## Error Responses

All errors return XML in S3-compatible format:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>NoSuchKey</Code>
  <Message>The specified key does not exist</Message>
  <Resource>/mybucket/nonexistent.txt</Resource>
  <RequestId>1634473200123456789</RequestId>
</Error>
```

### Common Error Codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `NoSuchKey` | 404 | Object does not exist |
| `NoSuchBucket` | 404 | Bucket does not exist |
| `BadDigest` | 400 | Content-MD5 header mismatch |
| `InvalidPart` | 400 | Invalid multipart upload part |
| `InvalidPartOrder` | 400 | Parts not uploaded in order |
| `InvalidRange` | 416 | Invalid byte range |
| `InvalidArgument` | 400 | Invalid query parameter |
| `AccessDenied` | 403 | Authentication failed or no permission |
| `SignatureDoesNotMatch` | 403 | Invalid signature |
| `SlowDown` | 503 | Rate limit exceeded |
| `InternalError` | 500 | Internal server error |
| `ServiceUnavailable` | 503 | Service temporarily unavailable |
| `MethodNotAllowed` | 405 | HTTP method not supported |

---

## Request Headers

### Standard Headers

| Header | Required | Description |
|--------|----------|-------------|
| `Authorization` | Yes* | Authentication credentials |
| `Content-Type` | Recommended | MIME type of the request body |
| `Content-Length` | Yes** | Size of the request body in bytes |
| `Content-MD5` | No | Base64-encoded MD5 hash for integrity |
| `Date` | Yes*** | Request date in RFC1123 format |
| `Host` | Yes | Target host |

\* Required unless using presigned URLs  
\** Required for PUT operations  
\*** Required for signature-based authentication

### Custom Headers

| Header | Description |
|--------|-------------|
| `x-amz-meta-*` | Custom object metadata |
| `x-amz-storage-class` | Storage class (not implemented) |
| `x-amz-server-side-encryption` | Encryption settings (not implemented) |

### Conditional Headers

| Header | Description |
|--------|-------------|
| `If-Modified-Since` | Return object only if modified after date |
| `If-None-Match` | Return object only if ETag doesn't match |
| `If-Match` | Return object only if ETag matches |
| `If-Unmodified-Since` | Return object only if not modified after date |

### Range Header

```
Range: bytes=0-1023           # First 1KB
Range: bytes=1024-2047        # Second 1KB
Range: bytes=-1024            # Last 1KB
Range: bytes=1024-            # From byte 1024 to end
```

---

## Response Headers

### Standard Headers

| Header | Description |
|--------|-------------|
| `Content-Type` | MIME type of the response |
| `Content-Length` | Size of the response body |
| `ETag` | MD5 hash of the object (quoted) |
| `Last-Modified` | Last modification date (RFC1123) |
| `Accept-Ranges` | Always `bytes` for objects |
| `x-amz-request-id` | Unique request identifier |

### Range Response Headers

| Header | Description |
|--------|-------------|
| `Content-Range` | Byte range returned (e.g., `bytes 0-1023/10240`) |

### Custom Metadata

All custom metadata headers (`x-amz-meta-*`) are returned in HEAD and GET responses.

---

## Status Codes

### Success Codes

| Code | Description |
|------|-------------|
| `200 OK` | Request successful |
| `204 No Content` | Request successful, no response body |
| `206 Partial Content` | Range request successful |

### Client Error Codes

| Code | Description |
|------|-------------|
| `400 Bad Request` | Invalid request parameters |
| `403 Forbidden` | Authentication failed or no permission |
| `404 Not Found` | Object or bucket not found |
| `405 Method Not Allowed` | HTTP method not supported |
| `409 Conflict` | Resource conflict |
| `416 Range Not Satisfiable` | Invalid byte range |

### Server Error Codes

| Code | Description |
|------|-------------|
| `500 Internal Server Error` | Server error |
| `503 Service Unavailable` | Service temporarily unavailable or rate limited |

---

## Rate Limiting

The system implements adaptive rate limiting:

- Default: 1000 requests/second per bucket
- Configurable: Set via `-initial_rate_limit` flag
- Adaptive: Automatically adjusts based on performance
- Headers: Check `X-RateLimit-*` headers in response

**Rate Limit Headers:**
```
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 850
X-RateLimit-Reset: 1634473260
```

When rate limited, you'll receive:
```
HTTP/1.1 503 Service Unavailable
Retry-After: 1

<Error>
  <Code>SlowDown</Code>
  <Message>Please reduce your request rate</Message>
</Error>
```

---

## Health & Monitoring Endpoints

### Health Check

**Endpoint:** `GET /health`

**Response:**
```
HTTP/1.1 200 OK
Content-Type: text/plain

OK
```

### Readiness Check

**Endpoint:** `GET /ready`

**Response:**
```
HTTP/1.1 200 OK
Content-Type: text/plain

READY
```

### Metrics

**Endpoint:** `GET /metrics` (port 9091)

**Response:** Prometheus-formatted metrics

**Example:**
```
# HELP s3_requests_total Total number of requests
# TYPE s3_requests_total counter
s3_requests_total{method="GET",operation="GetObject",status="200"} 12345

# HELP s3_request_duration_seconds Request duration
# TYPE s3_request_duration_seconds histogram
s3_request_duration_seconds_bucket{method="GET",operation="GetObject",le="0.005"} 1000
```

### Debug Variables

**Endpoint:** `GET /debug/vars`

**Response:** JSON with expvar metrics

---

## Performance Tips

1. **Use HEAD requests** to check existence before GET
2. **Enable caching** for frequently accessed objects
3. **Use multipart upload** for files > 100MB
4. **Use concurrent upload** for files > 1GB
5. **Use range requests** for large files
6. **Enable compression** for text-based content
7. **Batch operations** for bulk deletes/copies
8. **Monitor rate limits** to avoid throttling
9. **Use presigned URLs** for direct client access
10. **Enable lifecycle policies** for automatic cleanup

---

## Examples

### Python (boto3)

```python
import boto3

s3 = boto3.client(
    's3',
    endpoint_url='http://localhost:9000',
    aws_access_key_id='AKEXAMPLE001',
    aws_secret_access_key='your-secret-key'
)

# Upload object
s3.put_object(Bucket='mybucket', Key='file.txt', Body=b'Hello World')

# Download object
response = s3.get_object(Bucket='mybucket', Key='file.txt')
data = response['Body'].read()

# List objects
response = s3.list_objects_v2(Bucket='mybucket', Prefix='docs/')
for obj in response.get('Contents', []):
    print(obj['Key'])

# Delete object
s3.delete_object(Bucket='mybucket', Key='file.txt')
```

### Node.js (AWS SDK)

```javascript
const AWS = require('aws-sdk');

const s3 = new AWS.S3({
  endpoint: 'http://localhost:9000',
  accessKeyId: 'AKEXAMPLE001',
  secretAccessKey: 'your-secret-key',
  s3ForcePathStyle: true,
  signatureVersion: 'v4'
});

// Upload object
await s3.putObject({
  Bucket: 'mybucket',
  Key: 'file.txt',
  Body: 'Hello World'
}).promise();

// Download object
const data = await s3.getObject({
  Bucket: 'mybucket',
  Key: 'file.txt'
}).promise();

console.log(data.Body.toString());

// List objects
const list = await s3.listObjectsV2({
  Bucket: 'mybucket',
  Prefix: 'docs/'
}).promise();

list.Contents.forEach(obj => console.log(obj.Key));

// Delete object
await s3.deleteObject({
  Bucket: 'mybucket',
  Key: 'file.txt'
}).promise();
```

### cURL

```bash
# Upload
curl -X PUT http://localhost:9000/mybucket/file.txt \
  -H "Content-Type: text/plain" \
  -d "Hello World"

# Download
curl http://localhost:9000/mybucket/file.txt

# Metadata
curl -I http://localhost:9000/mybucket/file.txt

# List
curl "http://localhost:9000/mybucket?list-type=2"

# Delete
curl -X DELETE http://localhost:9000/mybucket/file.txt
```

---

## Versioning

**API Version:** 1.0.0  
**Last Updated:** October 20, 2025  
**Compatibility:** S3 API v2

---

## Support

For questions or issues:
- GitHub: https://github.com/iProDev/S3-Server
- Documentation: https://github.com/iProDev/S3-Server/README.md
- Email: chavroka[at]gmail.com

---

© 2025 S3 Storage Project
