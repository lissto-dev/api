# What is Lissto
  
Lissto is a DevEnv and DevEx(perience) platform that simplifies the development of applications on Kubernetes.
It bridges the gap between Docker Compose loved by developers and Kubernetes loved by DevOps and platform engineers.  
  
## Lissto API

The API part of the Lissto Platform.

### Architecture

This API follows clean architecture principles with clear separation of concerns:

```
api/
├── cmd/server/main.go           # Application entry point
├── internal/
│   ├── server/server.go         # Echo server setup with go-pkgz/auth
│   ├── middleware/              # Authentication and authorization middleware
│   ├── api/                     # API resource handlers and models
│   │   ├── stack/               # Stack resource handlers and models
│   │   └── blueprint/           # Blueprint resource handlers and models
│   └── k8s/client.go           # Kubernetes client wrapper
├── pkg/
│   ├── auth/                    # Role hierarchy and permissions
│   ├── config/                  # Configuration loading utilities
│   └── response/                # Standard API response helpers
└── api-keys.yaml               # API key configuration
```

### Features

- **Clean Architecture**: Separated concerns with resource-based organization
- **API Key Authentication**: File-based API key management with role assignment
- **Role-Based Access Control**: Simple role verification (admin, developer, user)
- **Echo Framework**: High-performance HTTP framework
- **go-pkgz/auth Integration**: JWT token generation and management
- **Kubernetes Integration**: Ready for K8s API interactions
- **Standardized Responses**: Consistent API response format

## Quick Start

### 1. Install Dependencies

```bash
go mod tidy
```

### 2. Configure API Keys

On first startup, the API will automatically generate an admin API key and store it in the Kubernetes secret `lissto-api-keys` in the global namespace. Check the logs for the generated admin key.

To create additional API keys, use the admin endpoint (see "API Key Management" section below).

### 3. Run the Server

```bash
go run cmd/server/main.go
```

The server will start on `http://localhost:8080`

## API Endpoints

### Health Check
- `GET /health` - Service health check (no auth required)

### API Key Management (Admin Only)
- `POST /api/v1/_internal/api-keys` - Create a new API key

**Create API Key Example:**

```bash
curl -X POST \
     -H "X-API-Key: your-admin-key-here" \
     -H "Content-Type: application/json" \
     -d '{
       "name": "john",
       "role": "user",
       "slack_user_id": "U123456"
     }' \
     http://localhost:8080/api/v1/_internal/api-keys
```

**Request Body:**
- `name` (required): Unique name/identifier for the API key
- `role` (required): One of `admin`, `deploy`, or `user`
- `slack_user_id` (optional): Slack user ID for future integration

**Response:**
```json
{
  "success": true,
  "message": "API key created",
  "data": {
    "api_key": "user-abc123def4567890...",
    "name": "john",
    "role": "user"
  }
}
```

**Notes:**
- API keys use role-based prefixes: `admin-`, `deploy-`, or `user-` followed by a random hex string
- The API key is only shown once in the response. Save it securely as it cannot be retrieved later

### Stack Management
- `GET /api/v1/stacks` - List all stacks
- `GET /api/v1/stacks/:id` - Get specific stack
- `POST /api/v1/stacks` - Create new stack
- `PUT /api/v1/stacks/:id` - Update stack
- `DELETE /api/v1/stacks/:id` - Delete stack

### Blueprint Management (Developer/Admin)
- `GET /api/v1/blueprints` - List all blueprints
- `GET /api/v1/blueprints/:id` - Get specific blueprint
- `POST /api/v1/blueprints` - Create new blueprint
- `PUT /api/v1/blueprints/:id` - Update blueprint
- `DELETE /api/v1/blueprints/:id` - Delete blueprint

## Authentication

All API endpoints (except `/health`) require authentication via API key:

```bash
curl -H "X-API-Key: your-api-key-here" http://localhost:8080/api/v1/stacks
```

## Role-Based Access Control

### Roles

- **admin**: Full access to all resources
- **developer**: Access to blueprint resources
- **user**: Limited access (can be extended)

### Route Protection

- **Stack routes**: Require `admin` role
- **Blueprint routes**: Require `developer` OR `admin` role

## Example Usage

### List Stacks (Admin)

```bash
curl -H "X-API-Key: admin-key-abc123def456" \
     http://localhost:8080/api/v1/stacks
```

### Create Blueprint (Developer)

```bash
curl -X POST \
     -H "X-API-Key: dev-key-xyz789uvw012" \
     -H "Content-Type: application/json" \
     -d '{
       "name": "my-blueprint",
       "description": "My custom blueprint",
       "version": "1.0.0",
       "template": {
         "nginx": "1.21",
         "node": "18"
       }
     }' \
     http://localhost:8080/api/v1/blueprints
```

### Health Check

```bash
curl http://localhost:8080/health
```

## Response Format

All API responses follow a consistent format:

```json
{
  "success": true,
  "message": "Operation completed successfully",
  "data": {
    // Response data here
  }
}
```

Error responses:

```json
{
  "success": false,
  "error": "Error message here"
}
```

## Development

### Adding New Resources

To add a new resource (e.g., `environment`):

1. Create `internal/environment/` directory
2. Add `models.go`, `handlers.go`, `routes.go`
3. Register routes in `internal/server/server.go`
4. Follow the same pattern as existing resources

### Extending Authentication

The current implementation uses Kubernetes secret-based API keys. To extend:

1. Modify `pkg/config/config.go` for different storage
2. Update `internal/middleware/auth.go` for new auth methods
3. API keys can be managed via the admin endpoint or stored in the Kubernetes secret

### Kubernetes Integration

The `internal/k8s/client.go` provides a wrapper around the Kubernetes client. To use:

1. Initialize the client in your handlers
2. Use the provided methods or extend with new operations
3. Handle K8s API errors appropriately

## Configuration

### Environment Variables

- `KUBECONFIG`: Path to kubeconfig file (optional, defaults to `~/.kube/config`)
- `API_KEYS_FILE`: Path to API keys file (defaults to `api-keys.yaml`)

### API Keys Storage

API keys are stored in a Kubernetes secret (`lissto-api-keys` in the global namespace) and can be managed via:

1. **Admin API Endpoint** (recommended): Use `POST /api/v1/_internal/api-keys` with an admin API key
2. **Kubernetes Secret**: Directly edit the secret if needed

The secret format follows the YAML structure:
- `role`: Required role name (`admin`, `deploy`, or `user`)
- `api_key`: Required API key string
- `name`: Optional user name for logging
- `slack_user_id`: Optional Slack user ID for integration

## Security Considerations

- API keys are stored in Kubernetes secrets and loaded on startup
- API keys are cached in memory and can be updated dynamically
- All API communications should use HTTPS in production
- Rotate API keys regularly
- Use strong, random API keys (automatically generated)
- Monitor API key usage
- Admin API keys have full access - protect them carefully

## Next Steps

1. **Database Integration**: Replace mock data with persistent storage
2. **Kubernetes CRDs**: Implement actual K8s resource management
3. **Rate Limiting**: Add per-API-key rate limiting
4. **Audit Logging**: Log all API operations
5. **Metrics**: Add Prometheus metrics
6. **Testing**: Add comprehensive test suite