# مرجع API - سیستم ذخیره‌سازی شیء سازگار با S3

<div align="center" dir="rtl">

**راهنمای جامع API برای سیستم ذخیره‌سازی S3**

وەشان 1.0.0

</div>

---

## 📋 فهرست مطالب

- [معرفی](#معرفی)
- [احراز هویت](#احراز-هویت)
- [API عملیات پایه S3](#api-عملیات-پایه-s3)
- [API نسخه‌بندی](#api-نسخه‌بندی)
- [API عملیات دسته‌ای](#api-عملیات-دسته‌ای)
- [API بارگذاری همزمان](#api-بارگذاری-همزمان)
- [API URLهای امضا شده](#api-urlهای-امضا-شده)
- [API چرخه حیات](#api-چرخه-حیات)
- [API اعتبارنامه](#api-اعتبارنامه)
- [کدهای خطا](#کدهای-خطا)
- [نمونه‌های کاربردی](#نمونه‌های-کاربردی)

---

## معرفی

این مستندات API جامع برای سیستم ذخیره‌سازی شیء سازگار با S3 است. تمام endpointها، پارامترها، و نمونه‌های درخواست/پاسخ را پوشش می‌دهد.

### URL پایه

```
http://localhost:9000
```

### فرمت درخواست/پاسخ

- **فرمت درخواست:** JSON، XML، یا form data
- **فرمت پاسخ:** JSON یا XML (بر اساس header Accept)
- **رمزگذاری کاراکتر:** UTF-8

---

## احراز هویت

### 1. احراز هویت HMAC-SHA256 (توصیه می‌شود)

احراز هویت مبتنی بر امضا برای امنیت بالا.

**هدرهای مورد نیاز:**

```http
Authorization: S3-HMAC-SHA256 AccessKey=<access_key>,Signature=<signature>
Date: <timestamp>
```

**محاسبه امضا:**

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

# مثال
secret_key = "your-secret-key"
method = "GET"
path = "/mybucket/myfile.txt"
date = datetime.utcnow().strftime("%Y%m%dT%H%M%SZ")

signature = calculate_signature(secret_key, method, path, date)
```

**نمونه درخواست:**

```bash
curl -X GET http://localhost:9000/mybucket/file.txt \
  -H "Authorization: S3-HMAC-SHA256 AccessKey=AKIAIOSFODNN7EXAMPLE,Signature=xYz123..." \
  -H "Date: 20250120T120000Z"
```

### 2. احراز هویت AWS SigV4

سازگار با AWS S3 SDK.

```python
import boto3

s3 = boto3.client(
    's3',
    endpoint_url='http://localhost:9000',
    aws_access_key_id='your-access-key',
    aws_secret_access_key='your-secret-key',
    region_name='us-east-1'
)

# بارگذاری فایل
s3.put_object(Bucket='mybucket', Key='file.txt', Body=b'Hello World')
```

### 3. احراز هویت Bearer Token

احراز هویت ساده برای استفاده سریع.

```bash
curl -X GET http://localhost:9000/mybucket/file.txt \
  -H "Authorization: Bearer your-token-here"
```

---

## API عملیات پایه S3

### PUT Object - بارگذاری شیء

بارگذاری یک شیء به سطل.

**Endpoint:**
```
PUT /{bucket}/{key}
```

**پارامترها:**
- `bucket` (path, required) - نام سطل
- `key` (path, required) - کلید شیء

**هدرها:**
- `Content-Type` (optional) - نوع محتوا
- `Content-Length` (required) - اندازه محتوا
- `Content-MD5` (optional) - چکسام MD5

**نمونه درخواست:**

```bash
curl -X PUT http://localhost:9000/documents/report.pdf \
  -H "Content-Type: application/pdf" \
  -H "Content-MD5: 1B2M2Y8AsgTpgAmY7PhCfg==" \
  --data-binary @report.pdf
```

**نمونه پاسخ (200 OK):**

```json
{
  "etag": "d41d8cd98f00b204e9800998ecf8427e",
  "version_id": "v1",
  "size": 1048576
}
```

---

### GET Object - دریافت شیء

دانلود یک شیء از سطل.

**Endpoint:**
```
GET /{bucket}/{key}
```

**پارامترهای Query:**
- `versionId` (optional) - شناسه نسخه خاص

**هدرها:**
- `Range` (optional) - دریافت محدوده بایت (مثال: `bytes=0-1023`)

**نمونه درخواست:**

```bash
# دریافت شیء کامل
curl -X GET http://localhost:9000/documents/report.pdf

# دریافت نسخه خاص
curl -X GET "http://localhost:9000/documents/report.pdf?versionId=v5"

# دریافت محدوده
curl -X GET http://localhost:9000/documents/large-file.zip \
  -H "Range: bytes=0-1048575"
```

**نمونه پاسخ (200 OK):**

```
Content-Type: application/pdf
Content-Length: 1048576
ETag: "d41d8cd98f00b204e9800998ecf8427e"
Last-Modified: Mon, 20 Oct 2025 10:30:00 GMT

[binary content]
```

---

### HEAD Object - دریافت فراداده

دریافت فراداده شیء بدون دانلود محتوا.

**Endpoint:**
```
HEAD /{bucket}/{key}
```

**پارامترهای Query:**
- `versionId` (optional) - شناسه نسخه

**نمونه درخواست:**

```bash
curl -I http://localhost:9000/documents/report.pdf
```

**نمونه پاسخ (200 OK):**

```
HTTP/1.1 200 OK
Content-Type: application/pdf
Content-Length: 1048576
ETag: "d41d8cd98f00b204e9800998ecf8427e"
Last-Modified: Mon, 20 Oct 2025 10:30:00 GMT
X-Object-Version: v3
```

---

### DELETE Object - حذف شیء

حذف یک شیء از سطل.

**Endpoint:**
```
DELETE /{bucket}/{key}
```

**پارامترهای Query:**
- `versionId` (optional) - حذف نسخه خاص

**نمونه درخواست:**

```bash
# حذف آخرین نسخه
curl -X DELETE http://localhost:9000/documents/old-report.pdf

# حذف نسخه خاص
curl -X DELETE "http://localhost:9000/documents/report.pdf?versionId=v2"
```

**نمونه پاسخ (204 No Content):**

```json
{
  "deleted": true,
  "version_id": "v3",
  "delete_marker": false
}
```

---

### LIST Objects V2 - لیست اشیاء

لیست اشیاء موجود در سطل.

**Endpoint:**
```
GET /{bucket}?list-type=2
```

**پارامترهای Query:**
- `prefix` (optional) - فیلتر بر اساس پیشوند
- `delimiter` (optional) - محدودکننده (معمولاً `/`)
- `max-keys` (optional) - حداکثر تعداد کلید (پیش‌فرض: 1000)
- `continuation-token` (optional) - توکن صفحه بعدی

**نمونه درخواست:**

```bash
# لیست تمام اشیاء
curl "http://localhost:9000/documents?list-type=2"

# لیست با پیشوند
curl "http://localhost:9000/documents?list-type=2&prefix=reports/2025/"

# صفحه‌بندی
curl "http://localhost:9000/documents?list-type=2&max-keys=100&continuation-token=token123"
```

**نمونه پاسخ (200 OK):**

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

### Multipart Upload - بارگذاری چندقسمتی

برای بارگذاری فایل‌های بزرگ (> 100MB).

#### 1. شروع بارگذاری

**Endpoint:**
```
POST /{bucket}/{key}?uploads
```

**نمونه درخواست:**

```bash
curl -X POST "http://localhost:9000/videos/movie.mp4?uploads" \
  -H "Content-Type: video/mp4"
```

**نمونه پاسخ:**

```json
{
  "upload_id": "upload-123456",
  "bucket": "videos",
  "key": "movie.mp4"
}
```

#### 2. بارگذاری قسمت‌ها

**Endpoint:**
```
PUT /{bucket}/{key}?uploadId={upload_id}&partNumber={part_number}
```

**نمونه درخواست:**

```bash
# بارگذاری قسمت 1
curl -X PUT "http://localhost:9000/videos/movie.mp4?uploadId=upload-123456&partNumber=1" \
  --data-binary @movie.part1

# بارگذاری قسمت 2
curl -X PUT "http://localhost:9000/videos/movie.mp4?uploadId=upload-123456&partNumber=2" \
  --data-binary @movie.part2
```

**نمونه پاسخ:**

```json
{
  "etag": "abc123def456",
  "part_number": 1
}
```

#### 3. تکمیل بارگذاری

**Endpoint:**
```
POST /{bucket}/{key}?uploadId={upload_id}
```

**بدنه درخواست:**

```json
{
  "parts": [
    {"part_number": 1, "etag": "abc123def456"},
    {"part_number": 2, "etag": "ghi789jkl012"}
  ]
}
```

**نمونه درخواست:**

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

**نمونه پاسخ:**

```json
{
  "bucket": "videos",
  "key": "movie.mp4",
  "etag": "final-etag-xyz",
  "location": "/videos/movie.mp4"
}
```

#### 4. لغو بارگذاری

**Endpoint:**
```
DELETE /{bucket}/{key}?uploadId={upload_id}
```

```bash
curl -X DELETE "http://localhost:9000/videos/movie.mp4?uploadId=upload-123456"
```

---

## API نسخه‌بندی

### فعال/غیرفعال کردن نسخه‌بندی

**Endpoint:**
```
PUT /{bucket}?versioning
```

**بدنه درخواست:**

```json
{
  "enabled": true,
  "max_versions": 10,
  "retention_days": 90
}
```

**نمونه درخواست:**

```bash
curl -X PUT "http://localhost:9000/documents?versioning" \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "max_versions": 10,
    "retention_days": 90
  }'
```

**نمونه پاسخ:**

```json
{
  "bucket": "documents",
  "versioning_enabled": true,
  "max_versions": 10,
  "retention_days": 90
}
```

---

### لیست نسخه‌های شیء

**Endpoint:**
```
GET /{bucket}/{key}?versions
```

**نمونه درخواست:**

```bash
curl "http://localhost:9000/documents/report.pdf?versions"
```

**نمونه پاسخ:**

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

### بازگرداندن نسخه حذف‌شده

**Endpoint:**
```
POST /{bucket}/{key}?restore
```

**پارامترهای Query:**
- `versionId` (optional) - بازگرداندن از نسخه خاص

**نمونه درخواست:**

```bash
# بازگرداندن از آخرین نسخه
curl -X POST "http://localhost:9000/documents/deleted-file.pdf?restore"

# بازگرداندن از نسخه خاص
curl -X POST "http://localhost:9000/documents/file.pdf?restore&versionId=v3"
```

**نمونه پاسخ:**

```json
{
  "restored": true,
  "version_id": "v6",
  "previous_version": "v3"
}
```

---

## API عملیات دسته‌ای

### حذف دسته‌ای

حذف چندین شیء به صورت همزمان.

**Endpoint:**
```
POST /batch
```

**بدنه درخواست:**

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

**نمونه درخواست:**

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

**نمونه پاسخ:**

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

### کپی دسته‌ای

کپی چندین شیء به صورت همزمان.

**بدنه درخواست:**

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

### جابجایی دسته‌ای

جابجایی (کپی + حذف) چندین شیء.

**بدنه درخواست:**

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

## API بارگذاری همزمان

### شروع بارگذاری همزمان

**Endpoint:**
```
POST /concurrent-upload/initiate
```

**بدنه درخواست:**

```json
{
  "bucket": "videos",
  "key": "movie.mp4",
  "total_size": 1073741824,
  "chunk_size": 10485760,
  "concurrency": 8
}
```

**نمونه درخواست:**

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

**نمونه پاسخ:**

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

### بارگذاری تکه

**Endpoint:**
```
PUT /concurrent-upload/chunk/{upload_id}/{chunk_index}
```

**نمونه درخواست:**

```bash
curl -X PUT "http://localhost:9000/concurrent-upload/chunk/concurrent-upload-789/0" \
  --data-binary @movie.chunk0
```

---

### تکمیل بارگذاری همزمان

**Endpoint:**
```
POST /concurrent-upload/complete/{upload_id}
```

**نمونه درخواست:**

```bash
curl -X POST "http://localhost:9000/concurrent-upload/complete/concurrent-upload-789"
```

**نمونه پاسخ:**

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

## API URLهای امضا شده

### تولید URL امضا شده

**Endpoint:**
```
POST /presign
```

**بدنه درخواست:**

```json
{
  "bucket": "documents",
  "key": "report.pdf",
  "operation": "GET",
  "expires": 3600
}
```

**نمونه درخواست:**

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

**نمونه پاسخ:**

```json
{
  "url": "http://localhost:9000/documents/report.pdf?signature=xyz123&expires=1640000000",
  "expires_at": "2025-10-20T11:30:00Z"
}
```

---

## API چرخه حیات

### تنظیم سیاست چرخه حیات

**Endpoint:**
```
PUT /{bucket}?lifecycle
```

**بدنه درخواست:**

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

**نمونه درخواست:**

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

### دریافت سیاست چرخه حیات

**Endpoint:**
```
GET /{bucket}?lifecycle
```

**نمونه پاسخ:**

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

## API اعتبارنامه

### ایجاد اعتبارنامه

**Endpoint:**
```
POST /credentials
```

**بدنه درخواست:**

```json
{
  "name": "my-app-credentials",
  "permissions": ["read", "write", "delete"]
}
```

**نمونه درخواست:**

```bash
curl -X POST http://localhost:9000/credentials \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-app-credentials",
    "permissions": ["read", "write"]
  }'
```

**نمونه پاسخ:**

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

### لیست اعتبارنامه‌ها

**Endpoint:**
```
GET /credentials
```

**نمونه پاسخ:**

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

### حذف اعتبارنامه

**Endpoint:**
```
DELETE /credentials/{access_key}
```

**نمونه درخواست:**

```bash
curl -X DELETE http://localhost:9000/credentials/AKIAIOSFODNN7EXAMPLE
```

---

## کدهای خطا

### کدهای وضعیت HTTP

| کد | وضعیت | توضیح |
|----|-------|-------|
| 200 | OK | عملیات موفق |
| 204 | No Content | حذف موفق |
| 400 | Bad Request | درخواست نامعتبر |
| 401 | Unauthorized | احراز هویت شکست خورد |
| 403 | Forbidden | دسترسی رد شد |
| 404 | Not Found | شیء پیدا نشد |
| 409 | Conflict | تداخل (مثلاً سطل موجود است) |
| 500 | Internal Server Error | خطای سرور |
| 503 | Service Unavailable | سرویس در دسترس نیست |

### کدهای خطای سفارشی

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

**کدهای خطای رایج:**

- `NoSuchBucket` - سطل وجود ندارد
- `NoSuchKey` - کلید وجود ندارد
- `InvalidArgument` - آرگومان نامعتبر
- `AccessDenied` - دسترسی رد شد
- `SignatureDoesNotMatch` - امضا مطابقت ندارد
- `InvalidAccessKeyId` - کلید دسترسی نامعتبر

---

## نمونه‌های کاربردی

### Python با Boto3

```python
import boto3
from botocore.client import Config

# پیکربندی
s3 = boto3.client(
    's3',
    endpoint_url='http://localhost:9000',
    aws_access_key_id='AKIAIOSFODNN7EXAMPLE',
    aws_secret_access_key='wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY',
    region_name='us-east-1',
    config=Config(signature_version='s3v4')
)

# بارگذاری فایل
with open('report.pdf', 'rb') as f:
    s3.put_object(
        Bucket='documents',
        Key='reports/2025/report.pdf',
        Body=f,
        ContentType='application/pdf'
    )

# دانلود فایل
response = s3.get_object(Bucket='documents', Key='reports/2025/report.pdf')
data = response['Body'].read()

# لیست اشیاء
response = s3.list_objects_v2(Bucket='documents', Prefix='reports/')
for obj in response['Contents']:
    print(f"{obj['Key']}: {obj['Size']} bytes")

# حذف فایل
s3.delete_object(Bucket='documents', Key='reports/2025/old-report.pdf')
```

---

### Node.js با AWS SDK

```javascript
const AWS = require('aws-sdk');
const fs = require('fs');

// پیکربندی
const s3 = new AWS.S3({
  endpoint: 'http://localhost:9000',
  accessKeyId: 'AKIAIOSFODNN7EXAMPLE',
  secretAccessKey: 'wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY',
  region: 'us-east-1',
  s3ForcePathStyle: true,
  signatureVersion: 'v4'
});

// بارگذاری فایل
async function uploadFile() {
  const fileContent = fs.readFileSync('report.pdf');
  
  const params = {
    Bucket: 'documents',
    Key: 'reports/2025/report.pdf',
    Body: fileContent,
    ContentType: 'application/pdf'
  };
  
  const result = await s3.putObject(params).promise();
  console.log('Upload successful:', result.ETag);
}

// دانلود فایل
async function downloadFile() {
  const params = {
    Bucket: 'documents',
    Key: 'reports/2025/report.pdf'
  };
  
  const result = await s3.getObject(params).promise();
  fs.writeFileSync('downloaded-report.pdf', result.Body);
  console.log('Download successful');
}

// لیست اشیاء
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

### cURL - نمونه‌های پیشرفته

**بارگذاری با پیشرفت:**

```bash
curl -X PUT http://localhost:9000/videos/large-video.mp4 \
  --data-binary @large-video.mp4 \
  --progress-bar \
  -H "Content-Type: video/mp4"
```

**دانلود با ادامه:**

```bash
curl -C - -O http://localhost:9000/videos/large-video.mp4
```

**بارگذاری چندقسمتی کامل:**

```bash
#!/bin/bash

BUCKET="videos"
KEY="movie.mp4"
FILE="movie.mp4"

# 1. شروع بارگذاری
UPLOAD_ID=$(curl -X POST "http://localhost:9000/${BUCKET}/${KEY}?uploads" | jq -r '.upload_id')

# 2. تقسیم و بارگذاری قسمت‌ها
split -b 10M "$FILE" part_
PART_NUM=1
for part in part_*; do
  ETAG=$(curl -X PUT "http://localhost:9000/${BUCKET}/${KEY}?uploadId=${UPLOAD_ID}&partNumber=${PART_NUM}" \
    --data-binary @"$part" | jq -r '.etag')
  echo "{\"part_number\": ${PART_NUM}, \"etag\": \"${ETAG}\"}" >> parts.json
  PART_NUM=$((PART_NUM + 1))
done

# 3. تکمیل بارگذاری
curl -X POST "http://localhost:9000/${BUCKET}/${KEY}?uploadId=${UPLOAD_ID}" \
  -H "Content-Type: application/json" \
  -d "{\"parts\": [$(cat parts.json | paste -sd,)]}"

# پاکسازی
rm part_* parts.json
```

---

## سوالات متداول API

### چگونه خطاها را مدیریت کنم؟

همیشه کد وضعیت HTTP را بررسی کنید و برای خطاهای 4xx و 5xx تلاش مجدد با backoff نمایی پیاده‌سازی کنید.

### حداکثر اندازه شیء چقدر است؟

5TB برای هر شیء، اما برای فایل‌های > 100MB از بارگذاری چندقسمتی استفاده کنید.

### آیا می‌توانم فراداده سفارشی اضافه کنم؟

بله، از هدرهای `X-Amz-Meta-*` استفاده کنید:

```bash
curl -X PUT http://localhost:9000/mybucket/file.txt \
  -H "X-Amz-Meta-Author: John Doe" \
  -H "X-Amz-Meta-Department: Engineering" \
  -d "content"
```

---

<div align="center">

**📚 برای اطلاعات بیشتر، به [README.md](README_fa.md) مراجعه کنید**

© 2025 پروژه ذخیره‌سازی S3

</div>
