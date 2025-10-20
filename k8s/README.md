# Kubernetes Deployment

This directory contains Kubernetes manifests for deploying the S3 storage system.

## Prerequisites

- Kubernetes cluster (v1.20+)
- kubectl configured
- Storage class for PersistentVolumeClaims
- (Optional) Ingress controller (nginx, traefik, etc.)
- (Optional) cert-manager for TLS certificates

## Quick Start

### 1. Create Namespace and ConfigMap

```bash
kubectl apply -f namespace.yaml
kubectl apply -f configmap.yaml
kubectl apply -f secret.yaml
```

### 2. Deploy Storage Nodes

```bash
kubectl apply -f statefulset-nodes.yaml
```

Wait for nodes to be ready:

```bash
kubectl wait --for=condition=ready pod -l app=s3-node -n s3-storage --timeout=300s
```

### 3. Deploy Gateway

```bash
kubectl apply -f deployment-gateway.yaml
```

Wait for gateway to be ready:

```bash
kubectl wait --for=condition=ready pod -l app=s3-gateway -n s3-storage --timeout=300s
```

### 4. (Optional) Enable Autoscaling

```bash
kubectl apply -f hpa.yaml
```

### 5. (Optional) Setup Ingress

Edit `ingress.yaml` to set your domain, then:

```bash
kubectl apply -f ingress.yaml
```

## Deploy Everything at Once

```bash
kubectl apply -f .
```

## Verify Deployment

```bash
# Check all pods
kubectl get pods -n s3-storage

# Check services
kubectl get svc -n s3-storage

# Check gateway logs
kubectl logs -f deployment/s3-gateway -n s3-storage

# Check node logs
kubectl logs -f statefulset/s3-node -n s3-storage
```

## Access the Service

### From within the cluster:

```
http://s3-gateway.s3-storage.svc.cluster.local:8080
```

### From outside (LoadBalancer):

```bash
# Get external IP
kubectl get svc s3-gateway -n s3-storage

# Use the EXTERNAL-IP shown
```

### From outside (Ingress):

Access via your configured domain (e.g., https://s3.example.com)

## Scaling

### Scale Gateway Horizontally

```bash
kubectl scale deployment s3-gateway -n s3-storage --replicas=3
```

### Scale Storage Nodes

```bash
kubectl scale statefulset s3-node -n s3-storage --replicas=5
```

**Note**: When scaling nodes, update the gateway deployment to include new node URLs.

## Configuration

### Update ConfigMap

Edit `configmap.yaml` and apply:

```bash
kubectl apply -f configmap.yaml
kubectl rollout restart deployment/s3-gateway -n s3-storage
```

### Update Secrets

```bash
kubectl create secret generic s3-secrets \
  --from-literal=node-auth-token='your-new-token' \
  --namespace=s3-storage \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl rollout restart statefulset/s3-node -n s3-storage
kubectl rollout restart deployment/s3-gateway -n s3-storage
```

## Storage

### PersistentVolumeClaims

Storage nodes use PVCs from the default storage class. To use a specific storage class:

```yaml
# In statefulset-nodes.yaml, under volumeClaimTemplates:
spec:
  storageClassName: your-storage-class
  accessModes: ["ReadWriteOnce"]
  resources:
    requests:
      storage: 10Gi
```

### Increase Storage

```bash
# Edit PVC size
kubectl edit pvc data-s3-node-0 -n s3-storage

# Verify resize
kubectl describe pvc data-s3-node-0 -n s3-storage
```

## Monitoring

### Metrics Endpoint

Gateway exposes metrics on port 9091:

```bash
kubectl port-forward svc/s3-gateway-metrics 9091:9091 -n s3-storage
```

Access metrics at: http://localhost:9091/metrics

### Prometheus Integration

The gateway service has annotations for Prometheus auto-discovery:

```yaml
annotations:
  prometheus.io/scrape: "true"
  prometheus.io/port: "9091"
  prometheus.io/path: "/metrics"
```

## Health Checks

All pods have liveness and readiness probes:

- Liveness: `/health` endpoint
- Readiness: `/ready` endpoint

## Resource Limits

### Current Settings

**Gateway:**
- Requests: 512Mi RAM, 500m CPU
- Limits: 2Gi RAM, 2000m CPU

**Nodes:**
- Requests: 256Mi RAM, 250m CPU
- Limits: 1Gi RAM, 1000m CPU

Adjust based on your workload in the respective YAML files.

## Troubleshooting

### Pods not starting

```bash
# Check pod status
kubectl describe pod <pod-name> -n s3-storage

# Check events
kubectl get events -n s3-storage --sort-by='.lastTimestamp'
```

### Gateway can't reach nodes

```bash
# Check if nodes are running
kubectl get pods -l app=s3-node -n s3-storage

# Test connectivity
kubectl exec -it deployment/s3-gateway -n s3-storage -- wget -O- http://s3-node-0.s3-node-headless:8080/health
```

### Storage issues

```bash
# Check PVCs
kubectl get pvc -n s3-storage

# Check PV status
kubectl get pv

# Describe PVC for details
kubectl describe pvc data-s3-node-0 -n s3-storage
```

## Cleanup

### Delete everything

```bash
kubectl delete namespace s3-storage
```

### Delete specific components

```bash
kubectl delete -f deployment-gateway.yaml
kubectl delete -f statefulset-nodes.yaml
kubectl delete -f configmap.yaml
kubectl delete -f secret.yaml
kubectl delete -f namespace.yaml
```

**Note**: This will delete all data. PVCs and PVs will be removed based on their reclaim policy.

## Production Considerations

1. **TLS/SSL**: Use cert-manager or manual certificates
2. **Network Policies**: Restrict pod-to-pod communication
3. **Resource Quotas**: Set namespace resource limits
4. **Backup**: Implement backup strategy for PVs
5. **Monitoring**: Deploy Prometheus/Grafana
6. **Logging**: Configure centralized logging
7. **Security**: Use Pod Security Policies/Standards
8. **High Availability**: Deploy across multiple zones

## Architecture

```
┌──────────────────────────────────────┐
│          Ingress/LoadBalancer        │
│         (External Access)            │
└────────────────┬─────────────────────┘
                 │
┌────────────────▼─────────────────────┐
│         s3-gateway Service           │
│         (LoadBalancer/ClusterIP)     │
└────────────────┬─────────────────────┘
                 │
┌────────────────▼─────────────────────┐
│      s3-gateway Deployment           │
│      (2+ replicas with HPA)          │
└────────┬─────────────────────────────┘
         │
         │ Backend Requests
         ▼
┌─────────────────────────────────────┐
│    s3-node-headless Service         │
│    (Headless for direct pod access) │
└────────┬────────────────────────────┘
         │
    ┌────┴────┬─────────┐
    ▼         ▼         ▼
┌────────┐┌────────┐┌────────┐
│ node-0 ││ node-1 ││ node-2 │
│  +PVC  ││  +PVC  ││  +PVC  │
└────────┘└────────┘└────────┘
```

## Support

For issues or questions:
- Check pod logs: `kubectl logs <pod-name> -n s3-storage`
- Review events: `kubectl get events -n s3-storage`
- Check service endpoints: `kubectl get endpoints -n s3-storage`
