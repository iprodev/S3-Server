#!/usr/bin/env node
/**
 * S3 Storage Node.js Client Example
 * Demonstrates authentication, presigned URLs, and API usage
 */

const crypto = require('crypto');
const https = require('https');
const http = require('http');
const { URL } = require('url');

class S3Client {
    /**
     * Initialize S3 client
     * @param {string} baseUrl - Base URL of the S3 gateway
     * @param {string} accessKey - Your access key
     * @param {string} secretKey - Your secret key
     */
    constructor(baseUrl, accessKey, secretKey) {
        this.baseUrl = baseUrl;
        this.accessKey = accessKey;
        this.secretKey = secretKey;
    }

    /**
     * Generate HMAC-SHA256 signature
     * @private
     */
    _signRequest(method, path, dateHeader) {
        const stringToSign = `${method}\n${path}\n${dateHeader}`;
        const signature = crypto
            .createHmac('sha256', this.secretKey)
            .update(stringToSign)
            .digest('hex');
        return signature;
    }

    /**
     * Generate request headers with authentication
     * @private
     */
    _getHeaders(method, path, contentType = null, contentMd5 = null) {
        const dateHeader = new Date().toUTCString();
        const signature = this._signRequest(method, path, dateHeader);

        const headers = {
            'Date': dateHeader,
            'Authorization': `S3-HMAC-SHA256 AccessKey=${this.accessKey},Signature=${signature}`
        };

        if (contentType) {
            headers['Content-Type'] = contentType;
        }

        if (contentMd5) {
            headers['Content-MD5'] = contentMd5;
        }

        return headers;
    }

    /**
     * Make HTTP request
     * @private
     */
    _request(method, path, headers, data = null) {
        return new Promise((resolve, reject) => {
            const url = new URL(path, this.baseUrl);
            const protocol = url.protocol === 'https:' ? https : http;

            const options = {
                hostname: url.hostname,
                port: url.port,
                path: url.pathname + url.search,
                method: method,
                headers: headers
            };

            const req = protocol.request(options, (res) => {
                const chunks = [];

                res.on('data', (chunk) => chunks.push(chunk));
                res.on('end', () => {
                    const body = Buffer.concat(chunks);
                    
                    if (res.statusCode >= 200 && res.statusCode < 300) {
                        resolve({
                            statusCode: res.statusCode,
                            headers: res.headers,
                            body: body
                        });
                    } else {
                        reject(new Error(`HTTP ${res.statusCode}: ${body.toString()}`));
                    }
                });
            });

            req.on('error', reject);

            if (data) {
                req.write(data);
            }

            req.end();
        });
    }

    /**
     * Upload an object
     * @param {string} bucket - Bucket name
     * @param {string} key - Object key
     * @param {Buffer|string} data - Object data
     * @param {string} contentType - Content type
     * @param {boolean} verifyMd5 - Calculate and send Content-MD5 header
     * @returns {Promise<Object>} Response object
     */
    async putObject(bucket, key, data, contentType = 'application/octet-stream', verifyMd5 = false) {
        const path = `/${bucket}/${key}`;

        // Convert string to buffer if needed
        const buffer = Buffer.isBuffer(data) ? data : Buffer.from(data);

        // Calculate MD5 if requested
        let contentMd5 = null;
        if (verifyMd5) {
            const md5 = crypto.createHash('md5').update(buffer).digest('base64');
            contentMd5 = md5;
        }

        const headers = this._getHeaders('PUT', path, contentType, contentMd5);
        headers['Content-Length'] = buffer.length;

        return await this._request('PUT', path, headers, buffer);
    }

    /**
     * Download an object
     * @param {string} bucket - Bucket name
     * @param {string} key - Object key
     * @param {Array<number>} byteRange - Optional [start, end] for range request
     * @returns {Promise<Buffer>} Object data
     */
    async getObject(bucket, key, byteRange = null) {
        const path = `/${bucket}/${key}`;
        const headers = this._getHeaders('GET', path);

        if (byteRange) {
            headers['Range'] = `bytes=${byteRange[0]}-${byteRange[1]}`;
        }

        const response = await this._request('GET', path, headers);
        return response.body;
    }

    /**
     * Get object metadata
     * @param {string} bucket - Bucket name
     * @param {string} key - Object key
     * @returns {Promise<Object>} Metadata object
     */
    async headObject(bucket, key) {
        const path = `/${bucket}/${key}`;
        const headers = this._getHeaders('HEAD', path);

        const response = await this._request('HEAD', path, headers);
        
        return {
            size: parseInt(response.headers['content-length'] || '0'),
            etag: (response.headers['etag'] || '').replace(/"/g, ''),
            contentType: response.headers['content-type'] || '',
            lastModified: response.headers['last-modified'] || ''
        };
    }

    /**
     * Delete an object
     * @param {string} bucket - Bucket name
     * @param {string} key - Object key
     * @returns {Promise<Object>} Response object
     */
    async deleteObject(bucket, key) {
        const path = `/${bucket}/${key}`;
        const headers = this._getHeaders('DELETE', path);

        return await this._request('DELETE', path, headers);
    }

    /**
     * List objects in a bucket
     * @param {string} bucket - Bucket name
     * @param {string} prefix - Key prefix filter
     * @param {number} maxKeys - Maximum number of objects to return
     * @returns {Promise<string>} XML response
     */
    async listObjects(bucket, prefix = '', maxKeys = 1000) {
        const path = `/${bucket}?list-type=2&prefix=${encodeURIComponent(prefix)}&max-keys=${maxKeys}`;
        const headers = this._getHeaders('GET', `/${bucket}`);

        const response = await this._request('GET', path, headers);
        return response.body.toString();
    }

    /**
     * Generate a presigned URL
     * @param {string} bucket - Bucket name
     * @param {string} key - Object key
     * @param {string} method - HTTP method (GET, PUT, DELETE)
     * @param {number} expiresIn - Expiration time in seconds
     * @returns {string} Presigned URL
     */
    generatePresignedUrl(bucket, key, method = 'GET', expiresIn = 3600) {
        const path = `/${bucket}/${key}`;
        const expires = Math.floor(Date.now() / 1000) + expiresIn;

        const stringToSign = `${method}\n${bucket}/${key}\n${expires}`;
        const signature = crypto
            .createHmac('sha256', this.secretKey)
            .update(stringToSign)
            .digest('hex');

        const url = new URL(path, this.baseUrl);
        url.searchParams.set('AWSAccessKeyId', this.accessKey);
        url.searchParams.set('Expires', expires.toString());
        url.searchParams.set('Signature', signature);

        return url.toString();
    }
}

/**
 * Example usage
 */
async function exampleUsage() {
    // Initialize client with your credentials
    const client = new S3Client(
        'http://localhost:9000',
        'AKEXAMPLE00000000001',
        'secretkey1234567890abcdefghijklmnopqrstuv'
    );

    console.log('S3 Storage Client Example');
    console.log('='.repeat(50));

    // Example 1: Upload an object
    console.log('\n1. Uploading object...');
    try {
        const response = await client.putObject(
            'mybucket',
            'test/hello.txt',
            'Hello, World!',
            'text/plain'
        );
        console.log(`   ✓ Upload successful! ETag: ${response.headers.etag}`);
    } catch (error) {
        console.log(`   ✗ Upload failed: ${error.message}`);
    }

    // Example 2: Download an object
    console.log('\n2. Downloading object...');
    try {
        const data = await client.getObject('mybucket', 'test/hello.txt');
        console.log(`   ✓ Downloaded: ${data.toString()}`);
    } catch (error) {
        console.log(`   ✗ Download failed: ${error.message}`);
    }

    // Example 3: Get object metadata
    console.log('\n3. Getting object metadata...');
    try {
        const metadata = await client.headObject('mybucket', 'test/hello.txt');
        console.log(`   ✓ Size: ${metadata.size} bytes`);
        console.log(`   ✓ ETag: ${metadata.etag}`);
        console.log(`   ✓ Content-Type: ${metadata.contentType}`);
    } catch (error) {
        console.log(`   ✗ Head request failed: ${error.message}`);
    }

    // Example 4: Range request
    console.log('\n4. Range request (first 5 bytes)...');
    try {
        const data = await client.getObject('mybucket', 'test/hello.txt', [0, 4]);
        console.log(`   ✓ Downloaded: ${data.toString()}`);
    } catch (error) {
        console.log(`   ✗ Range request failed: ${error.message}`);
    }

    // Example 5: Upload with MD5 verification
    console.log('\n5. Upload with MD5 verification...');
    try {
        await client.putObject(
            'mybucket',
            'test/verified.txt',
            'This upload is verified!',
            'text/plain',
            true
        );
        console.log(`   ✓ Verified upload successful!`);
    } catch (error) {
        console.log(`   ✗ Verified upload failed: ${error.message}`);
    }

    // Example 6: Generate presigned URL
    console.log('\n6. Generating presigned URL...');
    try {
        const presignedUrl = client.generatePresignedUrl(
            'mybucket',
            'test/hello.txt',
            'GET',
            3600  // 1 hour
        );
        console.log(`   ✓ Presigned URL (valid for 1 hour):`);
        console.log(`     ${presignedUrl}`);

        // Test the presigned URL
        const response = await fetch(presignedUrl);
        if (response.ok) {
            const text = await response.text();
            console.log(`   ✓ Presigned URL works! Data: ${text}`);
        }
    } catch (error) {
        console.log(`   ✗ Presigned URL generation failed: ${error.message}`);
    }

    // Example 7: List objects
    console.log('\n7. Listing objects with prefix...');
    try {
        const result = await client.listObjects('mybucket', 'test/');
        console.log(`   ✓ Listed objects (XML response)`);
    } catch (error) {
        console.log(`   ✗ List objects failed: ${error.message}`);
    }

    // Example 8: Delete object
    console.log('\n8. Deleting object...');
    try {
        await client.deleteObject('mybucket', 'test/verified.txt');
        console.log(`   ✓ Object deleted successfully`);
    } catch (error) {
        console.log(`   ✗ Delete failed: ${error.message}`);
    }

    console.log('\n' + '='.repeat(50));
    console.log('Examples complete!');
}

/**
 * Check metrics from the metrics endpoint
 */
async function checkMetrics() {
    console.log('\nMetrics Summary');
    console.log('='.repeat(50));

    try {
        const response = await fetch('http://localhost:9091/metrics');
        if (response.ok) {
            const text = await response.text();
            const lines = text.split('\n');

            const metricsOfInterest = [
                's3_requests_total',
                's3_auth_success_total',
                's3_auth_failure_total',
                's3_objects_stored_total',
                's3_bytes_stored_total'
            ];

            for (const metricName of metricsOfInterest) {
                const line = lines.find(l => l.startsWith(metricName) && !l.startsWith('#'));
                if (line) {
                    console.log(`  ${line}`);
                }
            }
        } else {
            console.log('  ✗ Could not fetch metrics');
        }
    } catch (error) {
        console.log(`  ✗ Error fetching metrics: ${error.message}`);
    }
}

// Run examples
if (require.main === module) {
    (async () => {
        await exampleUsage();
        await checkMetrics();
    })();
}

module.exports = S3Client;
