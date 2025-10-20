# سەرچاوەی API - سیستەمی هەڵگرتنی تەنی گونجاو لەگەڵ S3

<div align="center" dir="rtl">

**ڕێنمایی تەواوی API بۆ سیستەمی هەڵگرتنی S3**

وەشانی 1.0.0

</div>

---

## 📋 پێڕستی ناوەڕۆک

- [ناساندن](#ناساندن)
- [پشتڕاستکردنەوەی ناسنامە](#پشتڕاستکردنەوەی-ناسنامە)
- [API ـی کردارە بنەڕەتەکانی S3](#api-ـی-کردارە-بنەڕەتەکانی-s3)
- [API ـی وەشاندارکردن](#api-ـی-وەشاندارکردن)
- [API ـی کردارە دەستەییەکان](#api-ـی-کردارە-دەستەییەکان)
- [API ـی بارکردنی هاوکات](#api-ـی-بارکردنی-هاوکات)
- [API ـی URL ـە واژووکراوەکان](#api-ـی-url-ـە-واژووکراوەکان)
- [API ـی سووڕی ژیان](#api-ـی-سووڕی-ژیان)
- [API ـی ناسنامە](#api-ـی-ناسنامە)
- [کۆدەکانی هەڵە](#کۆدەکانی-هەڵە)
- [نموونە کاربەردییەکان](#نموونە-کاربەردییەکان)

---

## ناساندن

ئەم بەڵگەنامەیە API ـی تەواوە بۆ سیستەمی هەڵگرتنی تەنی گونجاو لەگەڵ S3. هەموو endpoint ـەکان، پارامەترەکان، و نموونەکانی داواکاری/وەڵام دەگرێتەوە.

### URL ـی بنەڕەت

```
http://localhost:9000
```

### فۆڕماتی داواکاری/وەڵام

- **فۆڕماتی داواکاری:** JSON، XML، یان form data
- **فۆڕماتی وەڵام:** JSON یان XML (لەسەر بنەمای سەردێڕی Accept)
- **کۆدکردنی نووسە:** UTF-8

---

## پشتڕاستکردنەوەی ناسنامە

### 1. پشتڕاستکردنەوەی HMAC-SHA256 (پێشنیار دەکرێت)

پشتڕاستکردنەوەی بنەمالەبەر لەسەر واژوو بۆ پاراستنی بەرز.

**سەردێڕە پێویستەکان:**

```http
Authorization: S3-HMAC-SHA256 AccessKey=<access_key>,Signature=<signature>
Date: <timestamp>
```

**ژمێریاری واژوو:**

```python
import hmac
import hashlib
import base64
from datetime import datetime

def calculate_signature(secret_key, method, path, date):
    string_to_sign = f"{method}\n{path}\n{date}"
    signature = hmac.new(
        secret_key.encode('utf-8'),
        string_to_sign.encode('utf-8'),
        hashlib.sha256
    ).digest()
    return base64.b64encode(signature).decode('utf-8')

# نموونە
secret_key = "your-secret-key"
method = "GET"
path = "/mybucket/myfile.txt"
date = datetime.utcnow().strftime("%Y%m%dT%H%M%SZ")

signature = calculate_signature(secret_key, method, path, date)
```

**نموونەی داواکاری:**

```bash
curl -X GET http://localhost:9000/mybucket/file.txt \
  -H "Authorization: S3-HMAC-SHA256 AccessKey=AKIAIOSFODNN7EXAMPLE,Signature=xYz123..." \
  -H "Date: 20250120T120000Z"
```

### 2. پشتڕاستکردنەوەی AWS SigV4

گونجاو لەگەڵ AWS S3 SDK.

```python
import boto3

s3 = boto3.client(
    's3',
    endpoint_url='http://localhost:9000',
    aws_access_key_id='your-access-key',
    aws_secret_access_key='your-secret-key',
    region_name='us-east-1'
)

# بارکردنی فایل
s3.put_object(Bucket='mybucket', Key='file.txt', Body=b'Hello World')
```

### 3. پشتڕاستکردنەوەی Bearer Token

پشتڕاستکردنەوەی سادە بۆ بەکارهێنانی خێرا.

```bash
curl -X GET http://localhost:9000/mybucket/file.txt \
  -H "Authorization: Bearer your-token-here"
```

---

## API ـی کردارە بنەڕەتەکانی S3

### PUT Object - بارکردنی تەن

بارکردنی تەنێک بۆ سەتڵ.

**Endpoint:**
```
PUT /{bucket}/{key}
```

**پارامەترەکان:**
- `bucket` (path, پێویست) - ناوی سەتڵ
- `key` (path, پێویست) - کلیلی تەن

**سەردێڕەکان:**
- `Content-Type` (دڵخواز) - جۆری ناوەڕۆک
- `Content-Length` (پێویست) - قەبارەی ناوەڕۆک
- `Content-MD5` (دڵخواز) - چێکسامی MD5

**نموونەی داواکاری:**

```bash
curl -X PUT http://localhost:9000/documents/report.pdf \
  -H "Content-Type: application/pdf" \
  -H "Content-MD5: 1B2M2Y8AsgTpgAmY7PhCfg==" \
  --data-binary @report.pdf
```

**نموونەی وەڵام (200 OK):**

```json
{
  "etag": "d41d8cd98f00b204e9800998ecf8427e",
  "version_id": "v1",
  "size": 1048576
}
```

---

### GET Object - وەرگرتنی تەن

داگرتنی تەنێک لە سەتڵ.

**Endpoint:**
```
GET /{bucket}/{key}
```

**پارامەترەکانی Query:**
- `versionId` (دڵخواز) - ناسنامەی وەشانی تایبەت

**سەردێڕەکان:**
- `Range` (دڵخواز) - وەرگرتنی مەودای بایت (نموونە: `bytes=0-1023`)

**نموونەی داواکاری:**

```bash
# وەرگرتنی تەنی تەواو
curl -X GET http://localhost:9000/documents/report.pdf

# وەرگرتنی وەشانی تایبەت
curl -X GET "http://localhost:9000/documents/report.pdf?versionId=v5"

# وەرگرتنی مەودا
curl -X GET http://localhost:9000/documents/large-file.zip \
  -H "Range: bytes=0-1048575"
```

**نموونەی وەڵام (200 OK):**

```
Content-Type: application/pdf
Content-Length: 1048576
ETag: "d41d8cd98f00b204e9800998ecf8427e"
Last-Modified: Mon, 20 Oct 2025 10:30:00 GMT

[binary content]
```

---

### HEAD Object - وەرگرتنی مێتاداتا

وەرگرتنی مێتاداتای تەن بێ داگرتنی ناوەڕۆک.

**Endpoint:**
```
HEAD /{bucket}/{key}
```

**پارامەترەکانی Query:**
- `versionId` (دڵخواز) - ناسنامەی وەشان

**نموونەی داواکاری:**

```bash
curl -I http://localhost:9000/documents/report.pdf
```

**نموونەی وەڵام (200 OK):**

```
HTTP/1.1 200 OK
Content-Type: application/pdf
Content-Length: 1048576
ETag: "d41d8cd98f00b204e9800998ecf8427e"
Last-Modified: Mon, 20 Oct 2025 10:30:00 GMT
X-Object-Version: v3
```

---

### DELETE Object - سڕینەوەی تەن

سڕینەوەی تەنێک لە سەتڵ.

**Endpoint:**
```
DELETE /{bucket}/{key}
```

**پارامەترەکانی Query:**
- `versionId` (دڵخواز) - سڕینەوەی وەشانی تایبەت

**نموونەی داواکاری:**

```bash
# سڕینەوەی دوایین وەشان
curl -X DELETE http://localhost:9000/documents/old-report.pdf

# سڕینەوەی وەشانی تایبەت
curl -X DELETE "http://localhost:9000/documents/report.pdf?versionId=v2"
```

**نموونەی وەڵام (204 No Content):**

```json
{
  "deleted": true,
  "version_id": "v3",
  "delete_marker": false
}
```

---

### LIST Objects V2 - لیستی تەنەکان

لیستی تەنە بەردەستەکان لە سەتڵ.

**Endpoint:**
```
GET /{bucket}?list-type=2
```

**پارامەترەکانی Query:**
- `prefix` (دڵخواز) - فلتەر لەسەر بنەمای پێشگر
- `delimiter` (دڵخواز) - سنوورکەر (ئاسایی `/`)
- `max-keys` (دڵخواز) - زۆرترین ژمارەی کلیل (بنەڕەت: 1000)
- `continuation-token` (دڵخواز) - تۆکێنی لاپەڕەی دواتر

**نموونەی داواکاری:**

```bash
# لیستی هەموو تەنەکان
curl "http://localhost:9000/documents?list-type=2"

# لیست لەگەڵ پێشگر
curl "http://localhost:9000/documents?list-type=2&prefix=reports/2025/"

# لاپەڕەکردن
curl "http://localhost:9000/documents?list-type=2&max-keys=100&continuation-token=token123"
```

**نموونەی وەڵام (200 OK):**

```json
{
  "name": "documents",
  "prefix": "",
  "max_keys": 1000,
  "is_truncated": false,
  "contents": [
    {
      "key": "report.pdf",
      "size": 1048576,
      "etag": "d41d8cd98f00b204e9800998ecf8427e",
      "last_modified": "2025-10-20T10:30:00Z",
      "storage_class": "STANDARD"
    },
    {
      "key": "data.csv",
      "size": 524288,
      "etag": "098f6bcd4621d373cade4e832627b4f6",
      "last_modified": "2025-10-19T15:20:00Z",
      "storage_class": "STANDARD"
    }
  ]
}
```

---

### Multipart Upload - بارکردنی چەندپارچەیی

بۆ بارکردنی فایلە گەورەکان (> 100MB).

#### 1. دەستپێکردنی بارکردن

**Endpoint:**
```
POST /{bucket}/{key}?uploads
```

**نموونەی داواکاری:**

```bash
curl -X POST "http://localhost:9000/videos/movie.mp4?uploads" \
  -H "Content-Type: video/mp4"
```

**نموونەی وەڵام:**

```json
{
  "upload_id": "upload-123456",
  "bucket": "videos",
  "key": "movie.mp4"
}
```

#### 2. بارکردنی پارچەکان

**Endpoint:**
```
PUT /{bucket}/{key}?uploadId={upload_id}&partNumber={part_number}
```

**نموونەی داواکاری:**

```bash
# بارکردنی پارچەی 1
curl -X PUT "http://localhost:9000/videos/movie.mp4?uploadId=upload-123456&partNumber=1" \
  --data-binary @movie.part1

# بارکردنی پارچەی 2
curl -X PUT "http://localhost:9000/videos/movie.mp4?uploadId=upload-123456&partNumber=2" \
  --data-binary @movie.part2
```

**نموونەی وەڵام:**

```json
{
  "etag": "abc123def456",
  "part_number": 1
}
```

#### 3. تەواوکردنی بارکردن

**Endpoint:**
```
POST /{bucket}/{key}?uploadId={upload_id}
```

**لەشی داواکاری:**

```json
{
  "parts": [
    {"part_number": 1, "etag": "abc123def456"},
    {"part_number": 2, "etag": "ghi789jkl012"}
  ]
}
```

**نموونەی داواکاری:**

```bash
curl -X POST "http://localhost:9000/videos/movie.mp4?uploadId=upload-123456" \
  -H "Content-Type: application/json" \
  -d '{
    "parts": [
      {"part_number": 1, "etag": "abc123def456"},
      {"part_number": 2, "etag": "ghi789jkl012"}
    ]
  }'
```

**نموونەی وەڵام:**

```json
{
  "bucket": "videos",
  "key": "movie.mp4",
  "etag": "final-etag-xyz",
  "location": "/videos/movie.mp4"
}
```

#### 4. هەڵوەشاندنەوەی بارکردن

**Endpoint:**
```
DELETE /{bucket}/{key}?uploadId={upload_id}
```

```bash
curl -X DELETE "http://localhost:9000/videos/movie.mp4?uploadId=upload-123456"
```

---

## API ـی وەشاندارکردن

### چالاک/ناچالاککردنی وەشاندارکردن

**Endpoint:**
```
PUT /{bucket}?versioning
```

**لەشی داواکاری:**

```json
{
  "enabled": true,
  "max_versions": 10,
  "retention_days": 90
}
```

**نموونەی داواکاری:**

```bash
curl -X PUT "http://localhost:9000/documents?versioning" \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "max_versions": 10,
    "retention_days": 90
  }'
```

**نموونەی وەڵام:**

```json
{
  "bucket": "documents",
  "versioning_enabled": true,
  "max_versions": 10,
  "retention_days": 90
}
```

---

### لیستی وەشانەکانی تەن

**Endpoint:**
```
GET /{bucket}/{key}?versions
```

**نموونەی داواکاری:**

```bash
curl "http://localhost:9000/documents/report.pdf?versions"
```

**نموونەی وەڵام:**

```json
{
  "bucket": "documents",
  "key": "report.pdf",
  "versions": [
    {
      "version_id": "v5",
      "size": 1048576,
      "etag": "abc123",
      "last_modified": "2025-10-20T10:30:00Z",
      "is_latest": true,
      "is_delete_marker": false
    },
    {
      "version_id": "v4",
      "size": 1024000,
      "etag": "def456",
      "last_modified": "2025-10-19T14:20:00Z",
      "is_latest": false,
      "is_delete_marker": false
    }
  ]
}
```

---

### گەڕاندنەوەی وەشانی سڕاوە

**Endpoint:**
```
POST /{bucket}/{key}?restore
```

**پارامەترەکانی Query:**
- `versionId` (دڵخواز) - گەڕاندنەوە لە وەشانی تایبەت

**نموونەی داواکاری:**

```bash
# گەڕاندنەوە لە دوایین وەشان
curl -X POST "http://localhost:9000/documents/deleted-file.pdf?restore"

# گەڕاندنەوە لە وەشانی تایبەت
curl -X POST "http://localhost:9000/documents/file.pdf?restore&versionId=v3"
```

**نموونەی وەڵام:**

```json
{
  "restored": true,
  "version_id": "v6",
  "previous_version": "v3"
}
```

---

## API ـی کردارە دەستەییەکان

### سڕینەوەی دەستەیی

سڕینەوەی چەند تەنێک بە هاوکات.

**Endpoint:**
```
POST /batch
```

**لەشی داواکاری:**

```json
{
  "operation": "delete",
  "operations": [
    {"bucket": "logs", "key": "2024/01/01/app.log"},
    {"bucket": "logs", "key": "2024/01/02/app.log"},
    {"bucket": "logs", "key": "2024/01/03/app.log"}
  ],
  "options": {
    "concurrency": 50,
    "dry_run": false
  }
}
```

**نموونەی داواکاری:**

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
      "concurrency": 50,
      "dry_run": false
    }
  }'
```

**نموونەی وەڵام:**

```json
{
  "total": 2,
  "successful": 2,
  "failed": 0,
  "duration_ms": 150,
  "results": [
    {
      "bucket": "logs",
      "key": "2024/01/01/app.log",
      "success": true
    },
    {
      "bucket": "logs",
      "key": "2024/01/02/app.log",
      "success": true
    }
  ]
}
```

---

### کۆپیکردنی دەستەیی

کۆپیکردنی چەند تەنێک بە هاوکات.

**لەشی داواکاری:**

```json
{
  "operation": "copy",
  "operations": [
    {
      "source_bucket": "backups",
      "source_key": "2024/data.csv",
      "dest_bucket": "archives",
      "dest_key": "2024/archived-data.csv"
    }
  ],
  "options": {
    "concurrency": 20,
    "dry_run": false
  }
}
```

---

### گواستنەوەی دەستەیی

گواستنەوە (کۆپیکردن + سڕینەوە) چەند تەنێک.

**لەشی داواکاری:**

```json
{
  "operation": "move",
  "operations": [
    {
      "source_bucket": "temp",
      "source_key": "upload.txt",
      "dest_bucket": "permanent",
      "dest_key": "data/upload.txt"
    }
  ],
  "options": {
    "concurrency": 10
  }
}
```

---

## API ـی بارکردنی هاوکات

### دەستپێکردنی بارکردنی هاوکات

**Endpoint:**
```
POST /concurrent-upload/initiate
```

**لەشی داواکاری:**

```json
{
  "bucket": "videos",
  "key": "movie.mp4",
  "total_size": 1073741824,
  "chunk_size": 10485760,
  "concurrency": 8
}
```

**نموونەی داواکاری:**

```bash
curl -X POST http://localhost:9000/concurrent-upload/initiate \
  -H "Content-Type: application/json" \
  -d '{
    "bucket": "videos",
    "key": "movie.mp4",
    "total_size": 1073741824,
    "chunk_size": 10485760,
    "concurrency": 8
  }'
```

**نموونەی وەڵام:**

```json
{
  "upload_id": "concurrent-upload-789",
  "total_chunks": 102,
  "chunk_size": 10485760,
  "urls": [
    "/concurrent-upload/chunk/concurrent-upload-789/0",
    "/concurrent-upload/chunk/concurrent-upload-789/1"
  ]
}
```

---

### بارکردنی پارچە

**Endpoint:**
```
PUT /concurrent-upload/chunk/{upload_id}/{chunk_index}
```

**نموونەی داواکاری:**

```bash
curl -X PUT "http://localhost:9000/concurrent-upload/chunk/concurrent-upload-789/0" \
  --data-binary @movie.chunk0
```

---

### تەواوکردنی بارکردنی هاوکات

**Endpoint:**
```
POST /concurrent-upload/complete/{upload_id}
```

**نموونەی داواکاری:**

```bash
curl -X POST "http://localhost:9000/concurrent-upload/complete/concurrent-upload-789"
```

**نموونەی وەڵام:**

```json
{
  "success": true,
  "bucket": "videos",
  "key": "movie.mp4",
  "size": 1073741824,
  "etag": "final-etag-abc",
  "duration_ms": 12500
}
```

---

## API ـی URL ـە واژووکراوەکان

### دروستکردنی URL ـی واژووکراو

**Endpoint:**
```
POST /presign
```

**لەشی داواکاری:**

```json
{
  "bucket": "documents",
  "key": "report.pdf",
  "operation": "GET",
  "expires": 3600
}
```

**نموونەی داواکاری:**

```bash
curl -X POST http://localhost:9000/presign \
  -H "Content-Type: application/json" \
  -d '{
    "bucket": "documents",
    "key": "report.pdf",
    "operation": "GET",
    "expires": 3600
  }'
```

**نموونەی وەڵام:**

```json
{
  "url": "http://localhost:9000/documents/report.pdf?signature=xyz123&expires=1640000000",
  "expires_at": "2025-10-20T11:30:00Z"
}
```

---

## API ـی سووڕی ژیان

### دانانی سیاسەتی سووڕی ژیان

**Endpoint:**
```
PUT /{bucket}?lifecycle
```

**لەشی داواکاری:**

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

**نموونەی داواکاری:**

```bash
curl -X PUT "http://localhost:9000/logs?lifecycle" \
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

---

### وەرگرتنی سیاسەتی سووڕی ژیان

**Endpoint:**
```
GET /{bucket}?lifecycle
```

**نموونەی وەڵام:**

```json
{
  "bucket": "logs",
  "rules": [
    {
      "id": "expire-old-logs",
      "prefix": "logs/",
      "enabled": true,
      "expiration_days": 30,
      "last_processed": "2025-10-20T09:00:00Z"
    }
  ]
}
```

---

## API ـی ناسنامە

### دروستکردنی ناسنامە

**Endpoint:**
```
POST /credentials
```

**لەشی داواکاری:**

```json
{
  "name": "my-app-credentials",
  "permissions": ["read", "write", "delete"]
}
```

**نموونەی داواکاری:**

```bash
curl -X POST http://localhost:9000/credentials \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-app-credentials",
    "permissions": ["read", "write"]
  }'
```

**نموونەی وەڵام:**

```json
{
  "access_key": "AKIAIOSFODNN7EXAMPLE",
  "secret_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
  "name": "my-app-credentials",
  "permissions": ["read", "write"],
  "created_at": "2025-10-20T10:00:00Z"
}
```

---

### لیستی ناسنامەکان

**Endpoint:**
```
GET /credentials
```

**نموونەی وەڵام:**

```json
{
  "credentials": [
    {
      "access_key": "AKIAIOSFODNN7EXAMPLE",
      "name": "my-app-credentials",
      "permissions": ["read", "write"],
      "created_at": "2025-10-20T10:00:00Z",
      "last_used": "2025-10-20T10:30:00Z"
    }
  ]
}
```

---

### سڕینەوەی ناسنامە

**Endpoint:**
```
DELETE /credentials/{access_key}
```

**نموونەی داواکاری:**

```bash
curl -X DELETE http://localhost:9000/credentials/AKIAIOSFODNN7EXAMPLE
```

---

## کۆدەکانی هەڵە

### کۆدەکانی دۆخی HTTP

| کۆد | دۆخ | ڕوونکردنەوە |
|----|-----|-------------|
| 200 | OK | کردار سەرکەوتوو |
| 204 | No Content | سڕینەوەی سەرکەوتوو |
| 400 | Bad Request | داواکاری نادروست |
| 401 | Unauthorized | پشتڕاستکردنەوە شکستی هێنا |
| 403 | Forbidden | دەستپێگەیشتن ڕەتکرایەوە |
| 404 | Not Found | تەن نەدۆزرایەوە |
| 409 | Conflict | پێکدادان (نموونە سەتڵ بەردەستە) |
| 500 | Internal Server Error | هەڵەی ڕاژەکار |
| 503 | Service Unavailable | خزمەتگوزاری بەردەست نییە |

### کۆدەکانی هەڵەی تایبەتمەند

```json
{
  "error": {
    "code": "NoSuchKey",
    "message": "The specified key does not exist",
    "resource": "/mybucket/nonexistent.txt",
    "request_id": "req-123456"
  }
}
```

**کۆدە باوەکانی هەڵە:**

- `NoSuchBucket` - سەتڵ بوونی نییە
- `NoSuchKey` - کلیل بوونی نییە
- `InvalidArgument` - ئارگیومێنتی نادروست
- `AccessDenied` - دەستپێگەیشتن ڕەتکرایەوە
- `SignatureDoesNotMatch` - واژوو گونجان نییە
- `InvalidAccessKeyId` - کلیلی دەستپێگەیشتنی نادروست

---

## نموونە کاربەردییەکان

### Python لەگەڵ Boto3

```python
import boto3
from botocore.client import Config

# ڕێکخستن
s3 = boto3.client(
    's3',
    endpoint_url='http://localhost:9000',
    aws_access_key_id='AKIAIOSFODNN7EXAMPLE',
    aws_secret_access_key='wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY',
    region_name='us-east-1',
    config=Config(signature_version='s3v4')
)

# بارکردنی فایل
with open('report.pdf', 'rb') as f:
    s3.put_object(
        Bucket='documents',
        Key='reports/2025/report.pdf',
        Body=f,
        ContentType='application/pdf'
    )

# داگرتنی فایل
response = s3.get_object(Bucket='documents', Key='reports/2025/report.pdf')
data = response['Body'].read()

# لیستی تەنەکان
response = s3.list_objects_v2(Bucket='documents', Prefix='reports/')
for obj in response['Contents']:
    print(f"{obj['Key']}: {obj['Size']} bytes")

# سڕینەوەی فایل
s3.delete_object(Bucket='documents', Key='reports/2025/old-report.pdf')
```

---

### Node.js لەگەڵ AWS SDK

```javascript
const AWS = require('aws-sdk');
const fs = require('fs');

// ڕێکخستن
const s3 = new AWS.S3({
  endpoint: 'http://localhost:9000',
  accessKeyId: 'AKIAIOSFODNN7EXAMPLE',
  secretAccessKey: 'wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY',
  region: 'us-east-1',
  s3ForcePathStyle: true,
  signatureVersion: 'v4'
});

// بارکردنی فایل
async function uploadFile() {
  const fileContent = fs.readFileSync('report.pdf');
  
  const params = {
    Bucket: 'documents',
    Key: 'reports/2025/report.pdf',
    Body: fileContent,
    ContentType: 'application/pdf'
  };
  
  const result = await s3.putObject(params).promise();
  console.log('بارکردن سەرکەوتوو:', result.ETag);
}

// داگرتنی فایل
async function downloadFile() {
  const params = {
    Bucket: 'documents',
    Key: 'reports/2025/report.pdf'
  };
  
  const result = await s3.getObject(params).promise();
  fs.writeFileSync('downloaded-report.pdf', result.Body);
  console.log('داگرتن سەرکەوتوو');
}

// لیستی تەنەکان
async function listObjects() {
  const params = {
    Bucket: 'documents',
    Prefix: 'reports/'
  };
  
  const result = await s3.listObjectsV2(params).promise();
  result.Contents.forEach(obj => {
    console.log(`${obj.Key}: ${obj.Size} bytes`);
  });
}
```

---

### cURL - نموونە پێشکەوتووەکان

**بارکردن لەگەڵ پێشکەوتن:**

```bash
curl -X PUT http://localhost:9000/videos/large-video.mp4 \
  --data-binary @large-video.mp4 \
  --progress-bar \
  -H "Content-Type: video/mp4"
```

**داگرتن لەگەڵ بەردەوامبوون:**

```bash
curl -C - -O http://localhost:9000/videos/large-video.mp4
```

**بارکردنی چەندپارچەیی تەواو:**

```bash
#!/bin/bash

BUCKET="videos"
KEY="movie.mp4"
FILE="movie.mp4"

# 1. دەستپێکردنی بارکردن
UPLOAD_ID=$(curl -X POST "http://localhost:9000/${BUCKET}/${KEY}?uploads" | jq -r '.upload_id')

# 2. دابەشکردن و بارکردنی پارچەکان
split -b 10M "$FILE" part_
PART_NUM=1
for part in part_*; do
  ETAG=$(curl -X PUT "http://localhost:9000/${BUCKET}/${KEY}?uploadId=${UPLOAD_ID}&partNumber=${PART_NUM}" \
    --data-binary @"$part" | jq -r '.etag')
  echo "{\"part_number\": ${PART_NUM}, \"etag\": \"${ETAG}\"}" >> parts.json
  PART_NUM=$((PART_NUM + 1))
done

# 3. تەواوکردنی بارکردن
curl -X POST "http://localhost:9000/${BUCKET}/${KEY}?uploadId=${UPLOAD_ID}" \
  -H "Content-Type: application/json" \
  -d "{\"parts\": [$(cat parts.json | paste -sd,)]}"

# پاککردنەوە
rm part_* parts.json
```

---

## پرسیارە باوەکانی API

### چۆن هەڵەکان بەڕێوەبەم؟

هەمیشە کۆدی دۆخی HTTP پشکنین بکە و بۆ هەڵەکانی 4xx و 5xx هەوڵدانەوە لەگەڵ backoff ـی نمایی جێبەجێ بکە.

### زۆرترین قەبارەی تەن چەندە؟

5TB بۆ هەر تەنێک، بەڵام بۆ فایلەکانی > 100MB لە بارکردنی چەندپارچەیی بەکاربهێنە.

### ئایا دەتوانم مێتاداتای تایبەتمەند زیاد بکەم؟

بەڵێ، لە سەردێڕەکانی `X-Amz-Meta-*` بەکاربهێنە:

```bash
curl -X PUT http://localhost:9000/mybucket/file.txt \
  -H "X-Amz-Meta-Author: John Doe" \
  -H "X-Amz-Meta-Department: Engineering" \
  -d "content"
```

---

<div align="center">

**📚 بۆ زانیاری زیاتر، سەردانی [README.md](README_ku.md) بکە**

© 2025 پرۆژەی هەڵگرتنی S3

</div>
