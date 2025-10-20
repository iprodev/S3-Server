#!/usr/bin/env python3
"""
S3 Storage Client Example
Demonstrates authentication, presigned URLs, and API usage
"""

import hmac
import hashlib
import base64
import requests
from datetime import datetime, timezone
from urllib.parse import urljoin, urlencode
import json

class S3Client:
    """Simple S3-compatible storage client with HMAC authentication"""
    
    def __init__(self, base_url, access_key, secret_key):
        """
        Initialize S3 client
        
        Args:
            base_url: Base URL of the S3 gateway (e.g., http://localhost:9000)
            access_key: Your access key
            secret_key: Your secret key
        """
        self.base_url = base_url.rstrip('/')
        self.access_key = access_key
        self.secret_key = secret_key
    
    def _sign_request(self, method, path, date_header):
        """Generate HMAC-SHA256 signature"""
        string_to_sign = f"{method}\n{path}\n{date_header}"
        signature = hmac.new(
            self.secret_key.encode('utf-8'),
            string_to_sign.encode('utf-8'),
            hashlib.sha256
        ).hexdigest()
        return signature
    
    def _get_headers(self, method, path, content_type=None, content_md5=None):
        """Generate request headers with authentication"""
        date_header = datetime.now(timezone.utc).strftime('%a, %d %b %Y %H:%M:%S GMT')
        signature = self._sign_request(method, path, date_header)
        
        headers = {
            'Date': date_header,
            'Authorization': f'S3-HMAC-SHA256 AccessKey={self.access_key},Signature={signature}'
        }
        
        if content_type:
            headers['Content-Type'] = content_type
        
        if content_md5:
            headers['Content-MD5'] = content_md5
        
        return headers
    
    def put_object(self, bucket, key, data, content_type='application/octet-stream', verify_md5=False):
        """
        Upload an object
        
        Args:
            bucket: Bucket name
            key: Object key
            data: Object data (bytes or string)
            content_type: Content type
            verify_md5: If True, calculate and send Content-MD5 header
        
        Returns:
            Response object
        """
        path = f"/{bucket}/{key}"
        url = urljoin(self.base_url, path)
        
        # Convert string to bytes if needed
        if isinstance(data, str):
            data = data.encode('utf-8')
        
        # Calculate MD5 if requested
        content_md5 = None
        if verify_md5:
            md5 = hashlib.md5(data).digest()
            content_md5 = base64.b64encode(md5).decode('utf-8')
        
        headers = self._get_headers('PUT', path, content_type, content_md5)
        
        response = requests.put(url, data=data, headers=headers)
        response.raise_for_status()
        
        return response
    
    def get_object(self, bucket, key, byte_range=None):
        """
        Download an object
        
        Args:
            bucket: Bucket name
            key: Object key
            byte_range: Optional tuple (start, end) for range request
        
        Returns:
            Object data as bytes
        """
        path = f"/{bucket}/{key}"
        url = urljoin(self.base_url, path)
        
        headers = self._get_headers('GET', path)
        
        if byte_range:
            start, end = byte_range
            headers['Range'] = f'bytes={start}-{end}'
        
        response = requests.get(url, headers=headers)
        response.raise_for_status()
        
        return response.content
    
    def head_object(self, bucket, key):
        """
        Get object metadata
        
        Args:
            bucket: Bucket name
            key: Object key
        
        Returns:
            Dict with metadata (size, etag, content-type, etc.)
        """
        path = f"/{bucket}/{key}"
        url = urljoin(self.base_url, path)
        
        headers = self._get_headers('HEAD', path)
        
        response = requests.head(url, headers=headers)
        response.raise_for_status()
        
        return {
            'size': int(response.headers.get('Content-Length', 0)),
            'etag': response.headers.get('ETag', '').strip('"'),
            'content_type': response.headers.get('Content-Type', ''),
            'last_modified': response.headers.get('Last-Modified', '')
        }
    
    def delete_object(self, bucket, key):
        """
        Delete an object
        
        Args:
            bucket: Bucket name
            key: Object key
        
        Returns:
            Response object
        """
        path = f"/{bucket}/{key}"
        url = urljoin(self.base_url, path)
        
        headers = self._get_headers('DELETE', path)
        
        response = requests.delete(url, headers=headers)
        response.raise_for_status()
        
        return response
    
    def list_objects(self, bucket, prefix='', max_keys=1000):
        """
        List objects in a bucket
        
        Args:
            bucket: Bucket name
            prefix: Key prefix filter
            max_keys: Maximum number of objects to return
        
        Returns:
            List of object keys
        """
        path = f"/{bucket}"
        params = {
            'list-type': '2',
            'prefix': prefix,
            'max-keys': max_keys
        }
        url = urljoin(self.base_url, path) + '?' + urlencode(params)
        
        headers = self._get_headers('GET', path)
        
        response = requests.get(url, headers=headers)
        response.raise_for_status()
        
        # Parse XML response (simplified - you might want to use xml.etree)
        # For now, just return raw response
        return response.text
    
    def generate_presigned_url(self, bucket, key, method='GET', expires_in=3600):
        """
        Generate a presigned URL (note: this should ideally be done server-side)
        
        Args:
            bucket: Bucket name
            key: Object key
            method: HTTP method (GET, PUT, DELETE)
            expires_in: Expiration time in seconds
        
        Returns:
            Presigned URL string
        """
        path = f"/{bucket}/{key}"
        expires = int(datetime.now(timezone.utc).timestamp()) + expires_in
        
        string_to_sign = f"{method}\n{bucket}/{key}\n{expires}"
        signature = hmac.new(
            self.secret_key.encode('utf-8'),
            string_to_sign.encode('utf-8'),
            hashlib.sha256
        ).hexdigest()
        
        params = {
            'AWSAccessKeyId': self.access_key,
            'Expires': expires,
            'Signature': signature
        }
        
        url = urljoin(self.base_url, path) + '?' + urlencode(params)
        return url


def example_usage():
    """Example usage of the S3 client"""
    
    # Initialize client with your credentials
    client = S3Client(
        base_url='http://localhost:9000',
        access_key='AKEXAMPLE00000000001',
        secret_key='secretkey1234567890abcdefghijklmnopqrstuv'
    )
    
    print("S3 Storage Client Example")
    print("=" * 50)
    
    # Example 1: Upload an object
    print("\n1. Uploading object...")
    try:
        response = client.put_object(
            bucket='mybucket',
            key='test/hello.txt',
            data='Hello, World!',
            content_type='text/plain'
        )
        print(f"   ✓ Upload successful! ETag: {response.headers.get('ETag')}")
    except Exception as e:
        print(f"   ✗ Upload failed: {e}")
    
    # Example 2: Download an object
    print("\n2. Downloading object...")
    try:
        data = client.get_object(bucket='mybucket', key='test/hello.txt')
        print(f"   ✓ Downloaded: {data.decode('utf-8')}")
    except Exception as e:
        print(f"   ✗ Download failed: {e}")
    
    # Example 3: Get object metadata
    print("\n3. Getting object metadata...")
    try:
        metadata = client.head_object(bucket='mybucket', key='test/hello.txt')
        print(f"   ✓ Size: {metadata['size']} bytes")
        print(f"   ✓ ETag: {metadata['etag']}")
        print(f"   ✓ Content-Type: {metadata['content_type']}")
    except Exception as e:
        print(f"   ✗ Head request failed: {e}")
    
    # Example 4: Range request
    print("\n4. Range request (first 5 bytes)...")
    try:
        data = client.get_object(
            bucket='mybucket',
            key='test/hello.txt',
            byte_range=(0, 4)
        )
        print(f"   ✓ Downloaded: {data.decode('utf-8')}")
    except Exception as e:
        print(f"   ✗ Range request failed: {e}")
    
    # Example 5: Upload with MD5 verification
    print("\n5. Upload with MD5 verification...")
    try:
        response = client.put_object(
            bucket='mybucket',
            key='test/verified.txt',
            data='This upload is verified!',
            verify_md5=True
        )
        print(f"   ✓ Verified upload successful!")
    except Exception as e:
        print(f"   ✗ Verified upload failed: {e}")
    
    # Example 6: Generate presigned URL
    print("\n6. Generating presigned URL...")
    try:
        presigned_url = client.generate_presigned_url(
            bucket='mybucket',
            key='test/hello.txt',
            method='GET',
            expires_in=3600  # 1 hour
        )
        print(f"   ✓ Presigned URL (valid for 1 hour):")
        print(f"     {presigned_url}")
        
        # Test the presigned URL
        response = requests.get(presigned_url)
        if response.ok:
            print(f"   ✓ Presigned URL works! Data: {response.text}")
    except Exception as e:
        print(f"   ✗ Presigned URL generation failed: {e}")
    
    # Example 7: List objects
    print("\n7. Listing objects with prefix...")
    try:
        result = client.list_objects(bucket='mybucket', prefix='test/')
        print(f"   ✓ Listed objects (XML response)")
    except Exception as e:
        print(f"   ✗ List objects failed: {e}")
    
    # Example 8: Delete object
    print("\n8. Deleting object...")
    try:
        client.delete_object(bucket='mybucket', key='test/verified.txt')
        print(f"   ✓ Object deleted successfully")
    except Exception as e:
        print(f"   ✗ Delete failed: {e}")
    
    print("\n" + "=" * 50)
    print("Examples complete!")


def check_metrics():
    """Check metrics from the metrics endpoint"""
    print("\nMetrics Summary")
    print("=" * 50)
    
    try:
        response = requests.get('http://localhost:9091/metrics')
        if response.ok:
            # Parse some key metrics
            lines = response.text.split('\n')
            
            metrics_of_interest = [
                's3_requests_total',
                's3_auth_success_total',
                's3_auth_failure_total',
                's3_objects_stored_total',
                's3_bytes_stored_total'
            ]
            
            for metric_name in metrics_of_interest:
                for line in lines:
                    if line.startswith(metric_name) and not line.startswith('#'):
                        print(f"  {line}")
                        break
        else:
            print("  ✗ Could not fetch metrics")
    except Exception as e:
        print(f"  ✗ Error fetching metrics: {e}")


if __name__ == '__main__':
    # Run examples
    example_usage()
    
    # Show metrics
    check_metrics()
