# S3 Storage Client Examples

This directory contains client library examples for interacting with the S3-compatible storage system using various programming languages.

## Available Examples

### Python Client (`python_client.py`)

A complete Python client with HMAC-SHA256 authentication support.

**Requirements:**
```bash
pip install requests
```

**Usage:**
```bash
# Make it executable
chmod +x python_client.py

# Run the examples
./python_client.py

# Or run with Python directly
python3 python_client.py
```

**Features:**
- ✅ PUT/GET/HEAD/DELETE operations
- ✅ HMAC-SHA256 authentication
- ✅ Presigned URL generation
- ✅ Range requests
- ✅ MD5 verification
- ✅ List objects
- ✅ Metrics checking

**Quick Example:**
```python
from python_client import S3Client

client = S3Client(
    base_url='http://localhost:9000',
    access_key='your-access-key',
    secret_key='your-secret-key'
)

# Upload
client.put_object('mybucket', 'file.txt', 'Hello World!')

# Download
data = client.get_object('mybucket', 'file.txt')
print(data.decode('utf-8'))

# Delete
client.delete_object('mybucket', 'file.txt')
```

### Node.js Client (`nodejs_client.js`)

A complete Node.js client with HMAC-SHA256 authentication support.

**Requirements:**
- Node.js 14+ (uses built-in fetch API for presigned URL testing)

**Usage:**
```bash
# Make it executable
chmod +x nodejs_client.js

# Run the examples
./nodejs_client.js

# Or run with Node
node nodejs_client.js
```

**Features:**
- ✅ PUT/GET/HEAD/DELETE operations
- ✅ HMAC-SHA256 authentication
- ✅ Presigned URL generation
- ✅ Range requests
- ✅ MD5 verification
- ✅ List objects
- ✅ Metrics checking
- ✅ Promise-based API

**Quick Example:**
```javascript
const S3Client = require('./nodejs_client');

const client = new S3Client(
    'http://localhost:9000',
    'your-access-key',
    'your-secret-key'
);

// Upload
await client.putObject('mybucket', 'file.txt', 'Hello World!');

// Download
const data = await client.getObject('mybucket', 'file.txt');
console.log(data.toString());

// Delete
await client.deleteObject('mybucket', 'file.txt');
```

## Configuration

Before running the examples, ensure:

1. **Server is running:**
   ```bash
   # Start storage nodes
   ./start_nodes.sh
   
   # Start gateway with authentication
   ./start_gateway.sh
   ```

2. **Create credentials:**
   ```bash
   ./manage_credentials.sh create "example-app" "read,write,delete"
   ```

3. **Update the examples with your credentials:**
   - Replace `AKEXAMPLE00000000001` with your access key
   - Replace `secretkey1234567890abcdefghijklmnopqrstuv` with your secret key

## Authentication

Both clients use HMAC-SHA256 signature authentication:

1. **String to Sign:**
   ```
   METHOD\n
   PATH\n
   DATE_HEADER
   ```

2. **Signature:**
   ```
   HMAC-SHA256(secret_key, string_to_sign)
   ```

3. **Authorization Header:**
   ```
   S3-HMAC-SHA256 AccessKey=<access_key>,Signature=<signature>
   ```

## Presigned URLs

Both clients can generate presigned URLs for temporary access:

**Python:**
```python
url = client.generate_presigned_url(
    bucket='mybucket',
    key='file.txt',
    method='GET',
    expires_in=3600  # 1 hour
)
```

**Node.js:**
```javascript
const url = client.generatePresignedUrl(
    'mybucket',
    'file.txt',
    'GET',
    3600  # 1 hour
);
```

The generated URL can be shared and used without additional authentication until it expires.

## Range Requests

Download only part of an object:

**Python:**
```python
# Download first 100 bytes
data = client.get_object('mybucket', 'largefile.bin', byte_range=(0, 99))
```

**Node.js:**
```javascript
// Download first 100 bytes
const data = await client.getObject('mybucket', 'largefile.bin', [0, 99]);
```

## MD5 Verification

Upload with Content-MD5 header for data integrity:

**Python:**
```python
client.put_object(
    bucket='mybucket',
    key='important.dat',
    data=data,
    verify_md5=True
)
```

**Node.js:**
```javascript
await client.putObject(
    'mybucket',
    'important.dat',
    data,
    'application/octet-stream',
    true  // verifyMd5
);
```

## Error Handling

Both clients raise exceptions on HTTP errors:

**Python:**
```python
try:
    client.get_object('mybucket', 'nonexistent.txt')
except Exception as e:
    print(f"Error: {e}")
```

**Node.js:**
```javascript
try {
    await client.getObject('mybucket', 'nonexistent.txt');
} catch (error) {
    console.log(`Error: ${error.message}`);
}
```

## Monitoring

Both examples include a function to check Prometheus metrics:

**Python:**
```python
check_metrics()  # Prints key metrics
```

**Node.js:**
```javascript
await checkMetrics();  // Prints key metrics
```

## Advanced Usage

### Multipart Upload

For large files, use multipart upload (not shown in basic examples):

1. **Initiate:** `POST /{bucket}/{key}?uploads`
2. **Upload Parts:** `PUT /{bucket}/{key}?partNumber=N&uploadId=ID`
3. **Complete:** `POST /{bucket}/{key}?uploadId=ID` with part ETags
4. **Abort:** `DELETE /{bucket}/{key}?uploadId=ID`

### Lifecycle Policies

Objects can be automatically expired based on age. Configure lifecycle policies on the server:

```json
{
  "bucket": "mybucket",
  "rules": [
    {
      "id": "auto-delete",
      "prefix": "temp/",
      "enabled": true,
      "expiration_days": 7
    }
  ]
}
```

## Testing

Run the built-in examples to verify your setup:

```bash
# Python
python3 python_client.py

# Node.js
node nodejs_client.js
```

Expected output:
- ✓ Upload successful
- ✓ Download successful
- ✓ Metadata retrieved
- ✓ Range request works
- ✓ Presigned URL generated
- ✓ Object deleted

## Troubleshooting

### Authentication Failures

**Problem:** Getting 403 Forbidden errors

**Solutions:**
1. Verify credentials are correct
2. Check server is running with `-auth_enabled=true`
3. Verify `auth.json` contains your credentials
4. Check server logs for authentication attempts

### Connection Refused

**Problem:** Cannot connect to server

**Solutions:**
1. Verify server is running: `curl http://localhost:9000/health`
2. Check firewall settings
3. Verify port is correct (default: 9000)

### Invalid Signature

**Problem:** Signature validation fails

**Solutions:**
1. Ensure clocks are synchronized
2. Verify secret key matches exactly
3. Check that no extra whitespace in credentials
4. Verify signature algorithm implementation

## Performance Tips

1. **Reuse client instances** - Don't create new clients for each request
2. **Use range requests** for large files
3. **Enable MD5 verification** for critical data
4. **Use presigned URLs** to offload authentication
5. **Implement connection pooling** for high-throughput scenarios

## Security Best Practices

1. **Never commit credentials** to version control
2. **Use environment variables** for credentials:
   ```bash
   export S3_ACCESS_KEY="your-key"
   export S3_SECRET_KEY="your-secret"
   ```
3. **Use HTTPS** in production
4. **Rotate credentials regularly**
5. **Set short expiration** for presigned URLs (1-15 minutes)
6. **Implement rate limiting** on the client side

## Next Steps

- Read `NEW_FEATURES.md` for complete feature documentation
- Check `QUICKSTART.md` for server setup
- Review `grafana-dashboard.json` for monitoring setup
- Implement retry logic for production use
- Add connection pooling for better performance
- Implement multipart upload for large files

## Support

For issues or questions:
1. Check server logs
2. Review documentation in parent directory
3. Test with `curl` first to isolate client issues
4. Check metrics endpoint for server health

## License

Same as parent project (MIT License)
